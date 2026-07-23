CREATE TABLE auth_login_nonces (
    nonce_hash  BYTEA PRIMARY KEY CHECK (octet_length(nonce_hash) = 32),
    provider    TEXT NOT NULL
                CHECK (provider ~ '^[a-z][a-z0-9_]{0,31}$'),
    binding_hash BYTEA CHECK (
        binding_hash IS NULL OR octet_length(binding_hash) = 32
    ),
    issued_at   TIMESTAMPTZ NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    CHECK (expires_at > issued_at),
    CHECK (used_at IS NULL OR used_at >= issued_at)
);

CREATE INDEX idx_auth_login_nonces_expiry
    ON auth_login_nonces(expires_at);

-- Login challenges are bearer values returned only to the client. As with
-- refresh tokens and Dojo nonces, PostgreSQL stores only their SHA-256 digest.
-- binding_hash additionally binds an OIDC state to PKCE verifier, redirect URI
-- and token nonce without persisting any of those plaintext values. Consume is
-- a single conditional UPDATE, so concurrent replay attempts cannot both create
-- authenticated sessions.
