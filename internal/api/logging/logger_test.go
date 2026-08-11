package logging

import "testing"

func TestNewValidLevels(
	t *testing.T,
) {
	levels := []string{
		"debug",
		"info",
		"warn",
		"error",
	}

	for _, level := range levels {
		t.Run(
			level,
			func(t *testing.T) {
				logger, err := New(level)
				if err != nil {
					t.Fatalf(
						"New(%q) returned error: %v",
						level,
						err,
					)
				}

				if logger == nil {
					t.Fatal(
						"expected logger, got nil",
					)
				}
			},
		)
	}
}

func TestNewRejectsUnknownLevel(
	t *testing.T,
) {
	_, err := New("verbose")
	if err == nil {
		t.Fatal(
			"expected error for invalid log level",
		)
	}
}
