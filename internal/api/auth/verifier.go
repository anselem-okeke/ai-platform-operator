package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Verifier interface {
	Verify(
		context.Context,
		string,
	) (Identity, error)
}

type OIDCVerifier struct {
	verifier *oidc.IDTokenVerifier
}

func NewOIDCVerifier(
	ctx context.Context,
	issuer string,
	audience string,
	caFile string,
) (*OIDCVerifier, error) {
	httpClient, err := newOIDCHTTPClient(
		caFile,
	)
	if err != nil {
		return nil, err
	}

	oidcContext := oidc.ClientContext(
		ctx,
		httpClient,
	)

	provider, err := oidc.NewProvider(
		oidcContext,
		issuer,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"discover OIDC provider: %w",
			err,
		)
	}

	verifier := provider.VerifierContext(
		oidcContext,
		&oidc.Config{
			ClientID: audience,
		},
	)

	return &OIDCVerifier{
		verifier: verifier,
	}, nil
}

func newOIDCHTTPClient(
	caFile string,
) (*http.Client, error) {
	caPEM, err := os.ReadFile(
		caFile,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read OIDC CA file %q: %w",
			caFile,
			err,
		)
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf(
			"load system certificate pool: %w",
			err,
		)
	}

	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	if ok := rootCAs.AppendCertsFromPEM(
		caPEM,
	); !ok {
		return nil, fmt.Errorf(
			"OIDC CA file %q contains no valid certificates",
			caFile,
		)
	}

	baseTransport, ok :=
		http.DefaultTransport.(*http.Transport)

	if !ok {
		return nil, fmt.Errorf(
			"default HTTP transport has unexpected type",
		)
	}

	transport := baseTransport.Clone()

	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig =
			&tls.Config{}
	} else {
		transport.TLSClientConfig =
			transport.TLSClientConfig.Clone()
	}

	transport.TLSClientConfig.RootCAs =
		rootCAs

	return &http.Client{
		Transport: transport,
	}, nil
}

func (v *OIDCVerifier) Verify(
	ctx context.Context,
	rawToken string,
) (Identity, error) {
	token, err := v.verifier.Verify(
		ctx,
		rawToken,
	)
	if err != nil {
		return Identity{}, fmt.Errorf(
			"verify JWT: %w",
			err,
		)
	}

	var claims Claims

	if err := token.Claims(
		&claims,
	); err != nil {
		return Identity{}, fmt.Errorf(
			"decode JWT claims: %w",
			err,
		)
	}

	if claims.Subject == "" {
		return Identity{}, fmt.Errorf(
			"JWT subject claim is missing",
		)
	}

	return Identity{
		Subject:           claims.Subject,
		PreferredUsername: claims.PreferredUsername,
		ClientID:          claims.AuthorizedParty,
		Roles:             claims.RealmAccess.Roles,
	}, nil
}
