package libzome

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/group/edwards25519"
	"go.dedis.ch/kyber/v3/util/random"
)

// altered from stock to use pointers
func ElGamalEncrypt(group kyber.Group, pubkey kyber.Point, message *[]byte, lastOffset int) (
	K, C kyber.Point, offset int, complete bool) {

	messageSize := group.Point().EmbedLen()
	if messageSize > len(*message) {
		messageSize = len(*message)
		complete = true
	}

	offsetMessage := (*message)[lastOffset : lastOffset+messageSize]

	// Embed the message (or as much of it as will fit) into a curve point.
	M := group.Point().Embed(offsetMessage, random.New())

	complete = offset+messageSize >= len(*message)
	offset = messageSize + lastOffset
	// ElGamal-encrypt the point to produce ciphertext (K,C).
	k := group.Scalar().Pick(random.New()) // ephemeral private key
	K = group.Point().Mul(k, nil)          // ephemeral DH public key
	S := group.Point().Mul(k, pubkey)      // ephemeral DH shared secret
	C = S.Add(S, M)                        // message blinded with secret
	return
}

func ElGamalDecrypt(group kyber.Group, prikey kyber.Scalar, K, C kyber.Point) (
	message []byte, err error) {

	// ElGamal-decrypt the ciphertext (K,C) to reproduce the message.
	S := group.Point().Mul(prikey, K) // regenerate shared secret
	M := group.Point().Sub(C, S)      // use to un-blind the message
	message, err = M.Data()           // extract the embedded data
	return
}

func newKp() (kyber.Scalar, kyber.Point) {
	suite := edwards25519.NewBlakeSHA256Ed25519()
	// Create a public/private keypair
	private := suite.Scalar().Pick(random.New())
	public := suite.Point().Mul(private, nil)
	return private, public
}

type SignSuite interface {
	kyber.Group
	kyber.Encoding
	kyber.XOFFactory
}

// A basic, verifiable signature
type basicSig struct {
	C kyber.Scalar // challenge
	R kyber.Scalar // response
}

func hashSchnorr(suite SignSuite, message []byte, p kyber.Point) kyber.Scalar {
	pb, _ := p.MarshalBinary()
	c := suite.XOF(pb)
	c.Write(message)
	return suite.Scalar().Pick(c)
}

func SchnorrSign(suite SignSuite, message []byte,
	privateKey kyber.Scalar) []byte {

	randomByte := make([]byte, 32)
	random.Bytes(randomByte, random.New())

	random := suite.XOF(randomByte)
	// Create random secret v and public point commitment T
	v := suite.Scalar().Pick(random)
	T := suite.Point().Mul(v, nil)

	// Create challenge c based on message and T
	c := hashSchnorr(suite, message, T)

	// Compute response r = v - x*c
	r := suite.Scalar()
	r.Mul(privateKey, c).Sub(v, r)

	// Return verifiable signature {c, r}
	// Verifier will be able to compute v = r + x*c
	// And check that hashElgamal for T and the message == c
	buf := bytes.Buffer{}
	sig := basicSig{c, r}
	_ = suite.Write(&buf, &sig)
	return buf.Bytes()
}

func SchnorrVerify(suite SignSuite, message []byte, publicKey kyber.Point,
	signatureBuffer []byte) (bool, error) {

	// Decode the signature
	buf := bytes.NewBuffer(signatureBuffer)
	sig := basicSig{}
	if err := suite.Read(buf, &sig); err != nil {
		return false, err
	}
	r := sig.R
	c := sig.C

	// Compute base**(r + x*c) == T
	var P, T kyber.Point
	P = suite.Point()
	T = suite.Point()
	T.Add(T.Mul(r, nil), P.Mul(c, publicKey))

	// Verify that the hash based on the message and T
	// matches the challange c from the signature
	c = hashSchnorr(suite, message, T)
	if !c.Equal(sig.C) {
		return false, nil
	}

	return true, nil
}

type MessagePod struct {
	sharedSecret kyber.Point
	ciphertext   kyber.Point
	index        int
}

//TODO: remake using streams
// https://github.com/reugn/go-streams/blob/7e7870067fc1/examples/fs/main.go
// https://github.com/reugn/go-streams/blob/7e7870067fc1/examples/ws/main.go

func (a *App) EcEncrypt(uuid string, input *[]byte, messageHook func(input MessagePod) error) error {
	suite := edwards25519.NewBlakeSHA256Ed25519()
	messageSize := suite.Point().EmbedLen()
	//check if we have a keypair for this uuid
	pubKey, ok := a.globalConfig.knownKeypairs[uuid]
	if !ok {
		return fmt.Errorf("no keypair found for uuid")
	}

	// ElGamal-encrypt a message using the public key.

	offset := 0
	completed := false

	for !completed {
		K, C, offsetRet, complete := ElGamalEncrypt(suite, pubKey.key, input, offset)
		err := messageHook(MessagePod{sharedSecret: K, ciphertext: C, index: offsetRet / messageSize})
		if err != nil {
			return err
		}
		offset = offsetRet
		completed = complete
	}

	return nil

}

func (a *App) EcDecrypt(pods []MessagePod) ([]byte, error) {

	//TODO: new function needed to address one pod at a time, assemble them into a cache, use index to reassemble,
	//rerequest broken pods

	suite := edwards25519.NewBlakeSHA256Ed25519()
	// Decrypt it using the corresponding private key.
	var message []byte
	for _, pod := range pods {
		// Decrypt the message using the private key
		m, err := ElGamalDecrypt(suite, a.globalConfig.PrivKeyHex, pod.sharedSecret, pod.ciphertext)
		if err != nil {
			return nil, err
		}
		message = append(message, m...)
	}
	return message, nil
}

func (a *App) EcSign(toSign []byte) (string, error) {
	suite := edwards25519.NewBlakeSHA256Ed25519()

	// Generate the signature
	M := []byte("Hello World!") // message we want to sign
	sig := SchnorrSign(suite, M, a.globalConfig.PrivKeyHex)
	return hex.Dump(sig), nil
}

func (a *App) EcCheckSig(toCheck []byte, sig string) (bool, error) {
	suite := edwards25519.NewBlakeSHA256Ed25519()
	sigHex, err := hex.DecodeString(sig)
	if err != nil {
		return false, err
	}
	isMatch, err := SchnorrVerify(suite, toCheck, a.globalConfig.PubKeyHex, sigHex)
	if err != nil {
		return false, err
	}
	return isMatch, nil
}
