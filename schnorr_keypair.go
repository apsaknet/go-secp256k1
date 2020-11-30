package secp256k1

// #include "./depend/secp256k1/include/secp256k1_extrakeys.h"
// #include "./depend/secp256k1/include/secp256k1_schnorrsig.h"
import "C"
import "C"
import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/pkg/errors"
	"unsafe"
)

// errZeroedKeyPair is the error returned when using a zeroed pubkey
var errZeroedKeyPair = errors.New("the key pair is zeroed, which isn't a valid SchnorrKeyPair")

// SchnorrKeyPair is a type representing a pair of Secp256k1 private and public keys.
// This can be used to create Schnorr signatures
type SchnorrKeyPair struct {
	keypair C.secp256k1_keypair
}

// SerializedPrivateKey is a byte array representing the storage representation of a SchnorrKeyPair
type SerializedPrivateKey [SerializedPrivateKeySize]byte

// String returns the SchnorrKeyPair as the hexadecimal string
func (key SerializedPrivateKey) String() string {
	return hex.EncodeToString(key[:])
}

// String returns the SchnorrKeyPair as the hexadecimal string
func (key SchnorrKeyPair) String() string {
	return key.SerializePrivateKey().String()
}

// DeserializePrivateKey returns a SchnorrKeyPair type from a 32 byte private key.
// will verify it's a valid private key(Group Order > key > 0)
func DeserializePrivateKey(data *SerializedPrivateKey) (*SchnorrKeyPair, error) {
	key := SchnorrKeyPair{}
	cPtrPrivateKey := (*C.uchar)(&data[0])

	ret := C.secp256k1_keypair_create(context, &key.keypair, cPtrPrivateKey)
	if ret != 1 {
		return nil, errors.New("invalid SchnorrKeyPair (zero or bigger than the group order)")
	}
	return &key, nil
}

// DeserializePrivateKeyFromSlice returns a SchnorrKeyPair type from a serialized private key slice.
// will verify that it's 32 byte and it's a valid private key(Group Order > key > 0)
func DeserializePrivateKeyFromSlice(data []byte) (key *SchnorrKeyPair, err error) {
	if len(data) != SerializedPrivateKeySize {
		return nil, errors.Errorf("invalid private key length got %d, expected %d", len(data),
			SerializedPrivateKeySize)
	}

	serializedKey := &SerializedPrivateKey{}
	copy(serializedKey[:], data)
	return DeserializePrivateKey(serializedKey)
}

// GeneratePrivateKey generates a random valid private key from `crypto/rand`
func GeneratePrivateKey() (key *SchnorrKeyPair, err error) {
	rawKey := SerializedPrivateKey{}
	for {
		n, err := rand.Read(rawKey[:])
		if err != nil || n != len(rawKey) {
			return nil, err
		}
		key, err = DeserializePrivateKey(&rawKey)
		if err == nil {
			return key, nil
		}
	}
}

// SerializePrivateKey returns the private key in the keypair.
func (key *SchnorrKeyPair) SerializePrivateKey() *SerializedPrivateKey {
	// TODO: Replace with upstream function when merged: https://github.com/bitcoin-core/secp256k1/pull/845
	ret := SerializedPrivateKey{}
	for i := 0; i < 32; i++ {
		ret[i] = byte(key.keypair.data[i])
	}
	return &ret
}

// Add a tweak to the public key by doing `key + tweak % Group Order` and adjust the pub/priv keys according to parity. this adds it in place.
// This is meant for creating BIP-32(HD) wallets
func (key *SchnorrKeyPair) Add(tweak [32]byte) error {
	if key.isZeroed() {
		return errors.WithStack(errZeroedKeyPair)
	}
	cPtrTweak := (*C.uchar)(&tweak[0])
	ret := C.secp256k1_keypair_xonly_tweak_add(context, &key.keypair, cPtrTweak)
	if ret != 1 {
		return errors.New("failed Adding to private key. Tweak is bigger than the order or the complement of the private key")
	}
	return nil
}

// SchnorrPublicKey generates a PublicKey for the corresponding private key.
func (key *SchnorrKeyPair) SchnorrPublicKey() (*SchnorrPublicKey, error) {
	pubkey, _, err := key.schnorrPublicKeyInternal()
	return pubkey, err
}

func (key *SchnorrKeyPair) schnorrPublicKeyInternal() (pubkey *SchnorrPublicKey, wasOdd bool, err error) {
	if key.isZeroed() {
		return nil, false, errors.WithStack(errZeroedKeyPair)
	}
	pubkey = &SchnorrPublicKey{}
	cParity := C.int(42)
	ret := C.secp256k1_keypair_xonly_pub(context, &pubkey.pubkey, &cParity, &key.keypair)
	if ret != 1 {
		return nil, false, errors.New("the keypair contains invalid data")
	}

	return pubkey, parityBitToBool(cParity), nil

}

// SchnorrSign creates a schnorr signature using the private key and the input hashed message.
// Notice: the [32] byte array *MUST* be a hash of a message.
func (key *SchnorrKeyPair) SchnorrSign(hash *Hash) (*SchnorrSignature, error) {
	var auxilaryRand [32]byte
	n, err := rand.Read(auxilaryRand[:])
	if err != nil || n != len(auxilaryRand) {
		return nil, err
	}
	return key.schnorrSignInternal(hash, &auxilaryRand)
}

func (key *SchnorrKeyPair) schnorrSignInternal(hash *Hash, auxiliaryRand *[32]byte) (*SchnorrSignature, error) {
	if key.isZeroed() {
		return nil, errors.WithStack(errZeroedKeyPair)
	}
	signature := SchnorrSignature{}
	cPtrSig := (*C.uchar)(&signature.signature[0])
	cPtrHash := (*C.uchar)(&hash[0])
	cPtrAux := unsafe.Pointer(auxiliaryRand)
	ret := C.secp256k1_schnorrsig_sign(context, cPtrSig, cPtrHash, &key.keypair, C.secp256k1_nonce_function_bip340, cPtrAux)
	if ret != 1 {
		return nil, errors.New("failed Signing. You should call `DeserializePrivateKey` before calling this")
	}
	return &signature, nil

}
func (key *SchnorrKeyPair) isZeroed() bool {
	return isZeroed(key.keypair.data[:32]) || isZeroed(key.keypair.data[32:64]) || isZeroed(key.keypair.data[64:])
}

func parityBitToBool(parity C.int) bool {
	switch parity {
	case 0:
		return false
	case 1:
		return true
	default:
		panic(fmt.Sprintf("should never happen, parity should always be 1 or 0, instead got: %d", int(parity)))
	}
}
