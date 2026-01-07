package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge"
	"go.woodpecker-ci.org/woodpecker/v3/server/forge/types"
	"go.woodpecker-ci.org/woodpecker/v3/server/model"
	"golang.org/x/oauth2"
)

var _ forge.Forge = (*config)(nil)

func (c *config) repoURL(repo string) string {
	return path.Join(c.url, repo)
}

func (c *config) cloneURL(repo string) string {
	return fmt.Sprintf("%s.git", c.repoURL(repo))
}

func (c *config) gitRepo(name string) (*git.Repository, error) {
	path := filepath.Join(c.repoDir, name+".git")
	slog.Debug("opening git repo", slog.String("path", path), slog.String("repo", name))
	gitRepo, err := git.PlainOpen(path)
	if err != nil {
		slog.Error("", slog.Any("err", err))
		return nil, fmt.Errorf("could not open repo %q at %q: %w", name, path, err)
	}
	return gitRepo, nil
}

// Name returns the string name of this driver
func (c *config) Name() string {
	slog.Info("Name")
	return "gitpecker"
}

// URL returns the root url of a configured forge
func (c *config) URL() string {
	slog.Info("URL")
	return c.url
}

// Login authenticates the session and returns the
// forge user details and the URL to redirect to if not authorized yet.
func (c *config) Login(ctx context.Context, r *types.OAuthRequest) (*model.User, string, error) {
	slog.Info("Login")
	provider, err := oidc.NewProvider(ctx, c.clientProvider)
	if err != nil {
		slog.Error("could not create OIDC provider", slog.Any("err", err))
		return nil, "", err
	}
	config := oauth2.Config{
		ClientID:     c.clientID,
		ClientSecret: c.clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  c.clientRedirect,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	redirectURL := config.AuthCodeURL(r.State)
	if r.Code == "" {
		return nil, redirectURL, nil
	}

	oauthToken, err := config.Exchange(ctx, r.Code)
	if err != nil {
		slog.Error("could not exchange oauth token", slog.Any("err", err))
		return nil, redirectURL, err
	}

	userInfo, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(oauthToken))
	if err != nil {
		slog.Error("could not get user info", slog.Any("err", err))
		return nil, redirectURL, err
	}

	login := userInfo.Profile
	if login == "" {
		login = userInfo.Email
	}

	return &model.User{
		AccessToken:   oauthToken.AccessToken,
		RefreshToken:  oauthToken.RefreshToken,
		Expiry:        oauthToken.Expiry.Unix(),
		Login:         login,
		Email:         userInfo.Email,
		ForgeRemoteID: model.ForgeRemoteID("gitpecker"),
		Avatar:        fmt.Sprintf("https://www.libravatar.org/avatar/%x", md5.Sum([]byte(userInfo.Email))),
	}, redirectURL, nil
}

// Auth authenticates the session and returns the forge user
// login for the given token and secret
func (c *config) Auth(ctx context.Context, token string, secret string) (string, error) {
	slog.Info("Auth")
	provider, err := oidc.NewProvider(ctx, c.clientProvider)
	if err != nil {
		slog.Error("could not create oidc provider", slog.Any("err", err))
		return "", err
	}
	userInfo, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
	if err != nil {
		slog.Error("could not get user info", slog.Any("err", err))
		return "", err
	}

	login := userInfo.Profile
	if login == "" {
		login = userInfo.Email
	}

	return login, nil
}

// Teams fetches a list of team memberships from the forge.
func (c *config) Teams(ctx context.Context, u *model.User, p *model.ListOptions) ([]*model.Team, error) {
	slog.Info("Teams")
	return []*model.Team{}, nil
}

func (c *config) toRepo(name string) *model.Repo {
	repo := &model.Repo{
		ForgeRemoteID: model.ForgeRemoteID(name),
		Owner:         c.Name(),
		Name:          name,
		FullName:      name,
		ForgeURL:      c.repoURL(name),
		Clone:         c.cloneURL(name),
		Branch:        "main",
		Perm: &model.Perm{
			Pull:  true,
			Push:  true,
			Admin: true,
		},
	}
	slog.Debug("toRepo", slog.String("repo", fmt.Sprintf("%+v", repo)))
	return repo
}

// Repo fetches the repository from the forge, preferred is using the ID, fallback is owner/name.
func (c *config) Repo(ctx context.Context, _ *model.User, remoteID model.ForgeRemoteID, _ string, name string) (*model.Repo, error) {
	slog.Info("Repo", slog.Any("remoteID", remoteID), slog.String("name", name))
	if remoteID.IsValid() {
		name = string(remoteID)
	}
	_, err := c.gitRepo(name)
	if err != nil {
		slog.Error("could not get git repo", slog.String("repo", name), slog.Any("err", err))
		return nil, err
	}
	return c.toRepo(name), nil
}

// Repos fetches a list of repos from the forge.
func (c *config) Repos(ctx context.Context, u *model.User, p *model.ListOptions) ([]*model.Repo, error) {
	slog.Info("Repos")
	if p.Page > 1 {
		return []*model.Repo{}, nil
	}
	entries, err := os.ReadDir(c.repoDir)
	if err != nil {
		slog.Error("could not read repo dir", slog.Any("err", err))
		return nil, fmt.Errorf("could not read repo dir: %w", err)
	}
	repos := make([]*model.Repo, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".git") {
			repo, err := c.Repo(ctx, nil, "", "", strings.TrimSuffix(entry.Name(), ".git"))
			if err != nil {
				slog.Error("could not get repo", slog.Any("err", err))
				return nil, err
			}
			repos = append(repos, repo)
		}
	}
	return repos, nil
}

// File fetches a file from the forge repository and returns it in string
// format.
func (c *config) File(ctx context.Context, u *model.User, r *model.Repo, b *model.Pipeline, f string) ([]byte, error) {
	slog.Info("File")
	repo, err := c.gitRepo(r.Name)
	if err != nil {
		slog.Error("could not get git repo", slog.Any("err", err))
		return nil, err
	}
	commit, err := repo.CommitObject(plumbing.NewHash(b.Commit))
	if err != nil {
		slog.Error("could not get commit object", slog.String("commit", b.Commit), slog.String("repo", r.Name), slog.Any("err", err))
		return nil, fmt.Errorf("could not get commit for %q: %w", b.Commit, err)
	}
	file, err := commit.File(f)
	if err != nil {
		slog.Error("could not get file", slog.Any("err", err))
		return nil, fmt.Errorf("could not get file %q: %w", f, err)
	}
	content, err := file.Contents()
	if err != nil {
		slog.Error("could not get file contents", slog.Any("err", err))
		return nil, fmt.Errorf("could not get file contents from %q: %w", f, err)
	}
	return []byte(content), nil
}

// Dir fetches a folder from the forge repository
func (c *config) Dir(ctx context.Context, u *model.User, r *model.Repo, b *model.Pipeline, f string) ([]*types.FileMeta, error) {
	slog.Info("Dir")
	repo, err := c.gitRepo(r.Name)
	if err != nil {
		slog.Error("could not get git repo", slog.Any("err", err))
		return nil, err
	}
	commit, err := repo.CommitObject(plumbing.NewHash(b.Commit))
	if err != nil {
		slog.Error("could not get commit object", slog.String("commit", b.Commit), slog.String("repo", r.Name), slog.Any("err", err))
		return nil, fmt.Errorf("could not get commit for %q: %w", b.Commit, err)
	}
	files, err := commit.Files()
	if err != nil {
		slog.Error("could not get commit files", slog.Any("err", err))
		return nil, fmt.Errorf("could not get files for %q: %w", b.Commit, err)
	}
	f = path.Clean(f)
	f += "/*"
	metas := make([]*types.FileMeta, 0)
	if err := files.ForEach(func(fi *object.File) error {
		if m, _ := filepath.Match(f, fi.Name); m {
			data, err := c.File(ctx, u, r, b, fi.Name)
			if err != nil {
				slog.Error("could not get file", slog.Any("err", err))
				return err
			}
			metas = append(metas, &types.FileMeta{
				Name: fi.Name,
				Data: data,
			})
		}
		return nil
	}); err != nil {
		slog.Error("could not iterate files", slog.Any("err", err))
		return nil, fmt.Errorf("problem while iterating over files: %w", err)
	}
	return metas, nil
}

// Status sends the commit status to the forge.
// An example would be the GitHub pull request status.
func (c *config) Status(ctx context.Context, u *model.User, r *model.Repo, b *model.Pipeline, _ *model.Workflow) error {
	slog.Info("Status")
	return nil
}

// Netrc returns a .netrc file that can be used to clone
// private repositories from a forge.
func (c *config) Netrc(u *model.User, r *model.Repo) (*model.Netrc, error) {
	slog.Info("Netrc")
	return &model.Netrc{
		Machine:  c.URL(),
		Login:    string(u.ForgeRemoteID),
		Password: "",
		Type:     model.ForgeTypeAddon,
	}, nil
}

// Activate activates a repository by creating the post-commit hook.
func (c *config) Activate(ctx context.Context, u *model.User, r *model.Repo, link string) error {
	slog.Info("Activate")
	return nil
}

// Deactivate deactivates a repository by removing all previously created
// post-commit hooks matching the given link.
func (c *config) Deactivate(ctx context.Context, u *model.User, r *model.Repo, link string) error {
	slog.Info("Deactivate")
	return nil
}

// Branches returns the names of all branches for the named repository.
func (c *config) Branches(ctx context.Context, u *model.User, r *model.Repo, p *model.ListOptions) ([]string, error) {
	slog.Info("Branches")
	if p.Page > 1 {
		return []string{}, nil
	}
	repo, err := c.gitRepo(r.Name)
	if err != nil {
		slog.Error("could not get git repo", slog.Any("err", err))
		return nil, err
	}
	branches, err := repo.Branches()
	if err != nil {
		slog.Error("could not get branches", slog.Any("err", err))
		return nil, fmt.Errorf("could not get branches: %w", err)
	}
	result := make([]string, 0)
	if err := branches.ForEach(func(r *plumbing.Reference) error {
		result = append(result, string(r.Name().Short()))
		return nil
	}); err != nil {
		slog.Error("could not iterate over branches", slog.Any("err", err))
		return nil, fmt.Errorf("problem while iterating over branches: %w", err)
	}
	return result, nil
}

// BranchHead returns the sha of the head (latest commit) of the specified branch
func (c *config) BranchHead(ctx context.Context, u *model.User, r *model.Repo, branch string) (*model.Commit, error) {
	slog.Info("BranchHead")
	repo, err := c.gitRepo(r.Name)
	if err != nil {
		slog.Error("could not get git repo", slog.Any("err", err))
		return nil, err
	}
	ref, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		slog.Error("could not get branch ref", slog.Any("err", err))
		return nil, fmt.Errorf("could not resolve branch reference for %q: %w", branch, err)
	}
	return &model.Commit{
		SHA:      ref.Hash().String(),
		ForgeURL: c.repoURL(r.Name),
	}, nil
}

// PullRequests returns all pull requests for the named repository.
func (c *config) PullRequests(ctx context.Context, u *model.User, r *model.Repo, p *model.ListOptions) ([]*model.PullRequest, error) {
	slog.Info("PullRequest")
	return []*model.PullRequest{}, nil
}

// Hook parses the post-commit hook from the Request body and returns the
// required data in a standard format.
func (c *config) Hook(ctx context.Context, r *http.Request) (repo *model.Repo, pipeline *model.Pipeline, err error) {
	slog.Info("Hook")
	return nil, nil, &types.ErrIgnoreEvent{
		Event:  "all",
		Reason: "gitpecker does not support hooks",
	}
}

// OrgMembership returns if user is member of organization and if user
// is admin/owner in that organization.
func (c *config) OrgMembership(ctx context.Context, u *model.User, org string) (*model.OrgPerm, error) {
	slog.Info("OrgMembership")
	return &model.OrgPerm{
		Member: true,
		Admin:  true,
	}, nil
}

// Org fetches the organization from the forge by name. If the name is a user an org with type user is returned.
func (c *config) Org(ctx context.Context, u *model.User, org string) (*model.Org, error) {
	slog.Info("Org")
	return &model.Org{
		Name:   c.Name(),
		IsUser: true,
	}, nil
}
