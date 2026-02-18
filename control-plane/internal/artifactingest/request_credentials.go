package artifactingest

import (
	"net/http"
	"strings"
)

func applyRequestCredentials(req *http.Request, creds Credentials, artifactoryMode bool) {
	if req == nil || creds.Values == nil {
		return
	}

	authSet := false
	if authz := strings.TrimSpace(creds.Get("authorization")); authz != "" {
		req.Header.Set("Authorization", authz)
		authSet = true
	}
	username := strings.TrimSpace(creds.Get("username"))
	password := creds.Get("password")
	if !authSet && username != "" {
		req.SetBasicAuth(username, password)
		authSet = true
	}

	if artifactoryMode {
		if !authSet {
			token := strings.TrimSpace(creds.Get("artifactory_token"))
			if token == "" {
				token = strings.TrimSpace(creds.Get("jfrog_access_token"))
			}
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}
		apiKey := strings.TrimSpace(creds.Get("artifactory_api_key"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(creds.Get("jfrog_api_key"))
		}
		if apiKey != "" {
			req.Header.Set("X-JFrog-Art-Api", apiKey)
		}
	}

	for key, value := range creds.Values {
		if !strings.HasPrefix(key, "header:") {
			continue
		}
		headerName := strings.TrimSpace(strings.TrimPrefix(key, "header:"))
		if headerName == "" {
			continue
		}
		req.Header.Set(headerName, value)
	}
}
