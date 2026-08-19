package preflight

import "github.com/jacu-dev/jacu-harness/internal/capability/verify"

// allowlistCheck deliberately delegates policy parsing and matching to verify.
// Pre-flight must not grow a second command-policy implementation.
func allowlistCheck(root string, argv []string) bool {
	config, err := verify.LoadConfig(root)
	if err != nil {
		return false
	}
	return verify.New(config).Check(argv) == nil
}

func configuredPathDirs(root string) []string {
	config, err := verify.LoadConfig(root)
	if err != nil {
		return nil
	}
	return append([]string{}, config.PathDirs...)
}
