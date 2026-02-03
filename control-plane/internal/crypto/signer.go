package crypto

import "time"

type CASigner struct {
	CA     *CA
	CAPEM  []byte
}

func (s *CASigner) SignDeviceCert(csrPEM []byte, deviceID string, validity time.Duration) ([]byte, string, error) {
	return SignCSR(s.CA, csrPEM, deviceID, validity)
}

func (s *CASigner) CACertPEM() []byte {
	return s.CAPEM
}
