package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configsDir() string {
	return "./test_configs"
}

func Test_ZkConfig(t *testing.T) {
	scenarios := map[string]struct {
		filePaths     []string
		expectedPanic string
		chain         string
		flag          string
	}{
		"Valid union config only": {
			filePaths: []string{
				filepath.Join(configsDir(), "valid-test-cfg.json"),
			},
			expectedPanic: "",
			chain:         "valid-test",
			flag:          "",
		},
		"Valid union config using flag": {
			filePaths: []string{
				filepath.Join(configsDir(), "valid-test-cfg.json"),
			},
			expectedPanic: "",
			chain:         "valid-test",
			flag:          filepath.Join(configsDir(), "valid-test-cfg.json"),
		},
		"Valid union config using flag but invalid path": {
			filePaths: []string{
				filepath.Join(configsDir(), "valid-test-cfg.json"),
			},
			expectedPanic: "could not open union config",
			chain:         "valid-test",
			flag:          filepath.Join(configsDir(), "valid-test-cfg.json"),
		},
		"Valid individual configs (old implementation)": {
			filePaths: []string{
				filepath.Join(configsDir(), "valid-test-allocs.json"),
				filepath.Join(configsDir(), "valid-test-chainspec.json"),
				filepath.Join(configsDir(), "valid-test-conf.json"),
			},
			expectedPanic: "",
			chain:         "valid-test",
			flag:          "",
		},
		"Union invalid config file": {
			filePaths: []string{
				filepath.Join(configsDir(), "invalid-test-cfg.json"),
			},
			expectedPanic: "could not parse",
			chain:         "invalid-test",
			flag:          "",
		},
		"Flag overrides bad config": {
			filePaths: []string{
				filepath.Join(configsDir(), "invalid-test-cfg.json"),
			},
			expectedPanic: "could not parse alloc for",
			chain:         "invalid-test",
			flag:          filepath.Join(configsDir(), "invalid-test-cfg.json"),
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					actualPanic, ok := r.(string)
					if !ok {
						t.Errorf("expected panic message to start with %v, but got non-string panic: %v", scenario.expectedPanic, r)
						return
					}
					if scenario.expectedPanic == "" && actualPanic != "" {
						t.Errorf("expected no panic but got %v", actualPanic)
					}
					if !strings.HasPrefix(actualPanic, scenario.expectedPanic) {
						t.Errorf("expected panic message to start with %v, got %v", scenario.expectedPanic, actualPanic)
					}
				}
			}()

			tempDir, err := os.MkdirTemp("", "testdir-*")
			if err != nil {
				t.Fatalf("could not create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			DynamicChainConfigPath = tempDir
			if scenario.flag != "" {
				ZkConfigPath = filepath.Join(tempDir, filepath.Base(scenario.flag))
			} else {
				ZkConfigPath = ""
			}

			for _, filePath := range scenario.filePaths {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("could not read file %q: %v", filePath, err)
				}

				if err = os.WriteFile(filepath.Join(tempDir, filepath.Base(filePath)), content, 0644); err != nil {
					t.Fatalf("could not write file: %v", err)
				}
			}

			_ = NewZKConfig(scenario.chain)
		})
	}
}
