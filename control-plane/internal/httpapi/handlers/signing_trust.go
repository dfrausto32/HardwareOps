package handlers

import (
	"log"

	"github.com/hardwareops/control-plane/internal/artifacttrust"
	"github.com/hardwareops/control-plane/internal/store"
)

func currentSigningTrust(logger *log.Logger, st store.Store) *artifacttrust.SigningTrustBundle {
	if st == nil {
		return nil
	}
	keys, err := st.ListTrustedSigningKeys(false)
	if err != nil {
		if logger != nil {
			logger.Printf("list trusted signing keys: %v", err)
		}
		return nil
	}
	bundle := artifacttrust.BuildSigningTrustBundle(keys)
	if len(bundle.Keys) == 0 {
		return nil
	}
	return &bundle
}
