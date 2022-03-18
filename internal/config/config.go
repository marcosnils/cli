package config

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"github.com/99designs/keyring"
	"github.com/pkg/errors"
	ps "github.com/planetscale/planetscale-go/planetscale"

	"github.com/mitchellh/go-homedir"
	exec "golang.org/x/sys/execabs"
)

const (
	defaultConfigPath = "~/.config/planetscale"
	projectConfigName = ".pscale.yml"
	configName        = "pscale.yml"
	keyringService    = "pscale"
	keyringKey        = "access-token"
	tokenFileMode     = 0o600
)

// Config is dynamically sourced from various files and environment variables.
type Config struct {
	AccessToken  string
	BaseURL      string
	Organization string

	ServiceTokenID string
	ServiceToken   string

	// Project Configuration
	Database string
	Branch   string
}

func New() (*Config, error) {
	accessToken, err := readAccessToken()
	if err != nil {
		return nil, err
	}

	return &Config{
		AccessToken: accessToken,
		BaseURL:     ps.DefaultBaseURL,
	}, nil
}

func (c *Config) IsAuthenticated() bool {
	return (c.ServiceToken != "" && c.ServiceTokenID != "") || c.AccessToken != ""
}

// NewClientFromConfig creates a PlaentScale API client from our configuration
func (c *Config) NewClientFromConfig(clientOpts ...ps.ClientOption) (*ps.Client, error) {
	opts := []ps.ClientOption{
		ps.WithBaseURL(c.BaseURL),
	}

	if c.ServiceToken != "" && c.ServiceTokenID != "" {
		opts = append(opts, ps.WithServiceToken(c.ServiceTokenID, c.ServiceToken))
	} else {
		opts = append(opts, ps.WithAccessToken(c.AccessToken))
	}
	opts = append(opts, clientOpts...)

	return ps.NewClient(opts...)
}

// ConfigDir is the directory for PlanetScale config.
func ConfigDir() (string, error) {
	dir, err := homedir.Expand(defaultConfigPath)
	if err != nil {
		return "", fmt.Errorf("can't expand path %q: %s", defaultConfigPath, err)
	}

	return dir, nil
}

// ProjectConfigPath returns the path of a configuration inside a Git
// repository.
func ProjectConfigPath() (string, error) {
	basePath, err := RootGitRepoDir()
	if err == nil {
		return path.Join(basePath, projectConfigName), nil
	}
	return path.Join("", projectConfigName), nil
}

func RootGitRepoDir() (string, error) {
	var tl = []string{"rev-parse", "--show-toplevel"}
	out, err := exec.Command("git", tl...).CombinedOutput()
	if err != nil {
		return "", errors.New("unable to find git root directory")
	}

	return string(strings.TrimSuffix(string(out), "\n")), nil
}

func ProjectConfigFile() string {
	return projectConfigName
}

func readAccessToken() (string, error) {
	ring, err := openKeyring()

	if errors.Is(err, keyring.ErrNoAvailImpl) {
		accessToken, tokenErr := readAccessTokenPath()
		return string(accessToken), tokenErr
	}

	item, err := ring.Get(keyringKey)
	if err == nil {
		return string(item.Data), nil
	}

	if errors.Is(err, keyring.ErrKeyNotFound) {
		// Migrate to keychain
		accessToken, tokenErr := readAccessTokenPath()
		if len(accessToken) > 0 && tokenErr == nil {
			return migrateAccessToken(ring, accessToken)
		}
		return "", nil
	}

	return "", err
}

func migrateAccessToken(ring keyring.Keyring, accessToken []byte) (string, error) {
	err := ring.Set(keyring.Item{
		Key:  keyringKey,
		Data: accessToken,
	})
	if err != nil {
		return "", err
	}
	path, err := accessTokenPath()
	if err != nil {
		return "", err
	}
	err = os.Remove(path)
	if err != nil {
		return "", err
	}
	return string(accessToken), nil
}

func WriteAccessToken(accessToken string) error {
	ring, err := openKeyring()

	if errors.Is(err, keyring.ErrNoAvailImpl) {
		return writeAccessTokenPath(accessToken)
	}

	return ring.Set(keyring.Item{
		Key:  keyringKey,
		Data: []byte(accessToken),
	})
}

func DeleteAccessToken() error {
	ring, err := openKeyring()

	if errors.Is(err, keyring.ErrNoAvailImpl) {
		return deleteAccessTokenPath()
	}

	return ring.Remove(keyringKey)
}

func openKeyring() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		AllowedBackends: []keyring.BackendType{
			keyring.SecretServiceBackend,
			keyring.KWalletBackend,
			keyring.KeychainBackend,
			keyring.WinCredBackend,
		},
		ServiceName:              keyringService,
		KeychainTrustApplication: true,
		KeychainSynchronizable:   true,
	})
}

func accessTokenPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	return path.Join(dir, keyringKey), nil
}

func readAccessTokenPath() ([]byte, error) {
	var accessToken []byte
	tokenPath, err := accessTokenPath()
	if err != nil {
		return nil, err
	}

	stat, err := os.Stat(tokenPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatal(err)
		}
		return nil, err
	} else {
		if stat.Mode()&^tokenFileMode != 0 {
			err = os.Chmod(tokenPath, tokenFileMode)
			if err != nil {
				log.Printf("Unable to change %v file mode to 0%o: %v", tokenPath, tokenFileMode, err)
			}
		}
		accessToken, err = ioutil.ReadFile(tokenPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	return accessToken, nil
}

func deleteAccessTokenPath() error {
	tokenPath, err := accessTokenPath()
	if err != nil {
		return err
	}

	err = os.Remove(tokenPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "error removing access token file")
		}
	}

	configFile, err := DefaultConfigPath()
	if err != nil {
		return err
	}

	err = os.Remove(configFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "error removing default config file")
		}
	}
	return nil
}

func writeAccessTokenPath(accessToken string) error {
	configDir, err := ConfigDir()
	if err != nil {
		return err
	}

	_, err = os.Stat(configDir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(configDir, 0771)
		if err != nil {
			return errors.Wrap(err, "error creating config directory")
		}
	} else if err != nil {
		return err
	}

	tokenPath, err := accessTokenPath()
	if err != nil {
		return err
	}

	tokenBytes := []byte(accessToken)
	err = ioutil.WriteFile(tokenPath, tokenBytes, tokenFileMode)
	if err != nil {
		return errors.Wrap(err, "error writing token")
	}

	return nil
}
