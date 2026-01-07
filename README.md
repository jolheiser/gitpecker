# gitpecker

Woodpecker addon forge for plain (bare) git repositories using generic OIDC for authentication.

## Setup

Follow the [addon docs](https://woodpecker-ci.org/docs/administration/configuration/forges/addon) and define the following env vars for Woodpecker.

- `WOODPECKER_ADDON_FORGE` - Path to gitpecker binary
- `GITPECKER_REPOS` - The location on disk that stores the **bare** `.git` repos.
- `GITPECKER_URL` - The URL (if any) to see the git repos. (gitweb, cgit, etc.)
- `GITPECKER_PROVIDER` - OIDC provider URL
- `GITPECKER_CLIENT_ID` - OIDC Client ID
- `GITPECKER_CLIENT_SECRET` - OIDC Client Secret (prefer `GITPECKER_CLIENT_SECRET_FILE` where possible)
- `GITPECKER_CLIENT_SECRET_FILE` - Path to a file containing OIDC Client Secret
- `GITPECKER_REDIRECT` - Woodpecker authorize URL (e.g. `http://localhost:8000/authorize`)

## License

[MIT](LICENSE)
