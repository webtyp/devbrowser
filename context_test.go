package devbrowser

import (
	"reflect"
	"testing"
	"unsafe"

	"webtyp.com/devbrowser/chromedp"
)

// The value a consumer actually passes comes from httpd.DevCertSPKI(). This
// test uses a fixed 44-character base64 string of the same shape, asserts it
// reaches the allocator verbatim, and asserts the flag name is the SPKI list
// and not a blanket bypass.

func parseExecAllocatorOptions(opts []chromedp.ExecAllocatorOption) map[string]any {
	alloc := &chromedp.ExecAllocator{}

	// Initialize alloc.initFlags (unexported map[string]interface{})
	v := reflect.ValueOf(alloc).Elem()
	field := v.FieldByName("initFlags")
	initFlags := make(map[string]interface{})
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(initFlags))

	for _, opt := range opts {
		opt(alloc)
	}

	flags := make(map[string]any)
	for k, val := range initFlags {
		flags[k] = val
	}
	return flags
}

func TestBuildAllocatorOptions_SPKI(t *testing.T) {
	t.Run("empty TrustDevCertSPKI -> no spki flag", func(t *testing.T) {
		b := &DevBrowser{}
		opts := b.buildAllocatorOptions()
		flags := parseExecAllocatorOptions(opts)

		if val, exists := flags[FlagSPKIList]; exists {
			t.Errorf("expected no %s flag, but found: %v", FlagSPKIList, val)
		}
	})

	t.Run("set TrustDevCertSPKI -> exactly spki list flag with value", func(t *testing.T) {
		testPin := "47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="
		b := &DevBrowser{
			TrustDevCertSPKI: testPin,
		}
		opts := b.buildAllocatorOptions()
		flags := parseExecAllocatorOptions(opts)

		val, exists := flags[FlagSPKIList]
		if !exists {
			t.Fatalf("expected flag %s to be set, but it was missing", FlagSPKIList)
		}
		if val != testPin {
			t.Errorf("expected flag %s value %q, got %q", FlagSPKIList, testPin, val)
		}
	})

	t.Run("security regression guard -> no blanket bypass flags", func(t *testing.T) {
		b := &DevBrowser{
			TrustDevCertSPKI: "47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=",
		}
		opts := b.buildAllocatorOptions()
		flags := parseExecAllocatorOptions(opts)

		forbiddenFlags := []string{
			"ignore-certificate-errors",
			"allow-insecure-localhost",
			"disable-web-security",
		}

		for _, flag := range forbiddenFlags {
			if val, exists := flags[flag]; exists {
				t.Errorf("security regression: forbidden flag %q was set with value %v", flag, val)
			}
		}
	})
}
