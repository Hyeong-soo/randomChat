package profile

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestPlaceholderAvatar(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantChar string
	}{
		{"normal username", "alice", "a"},
		{"single char", "b", "b"},
		{"empty username", "", "?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := placeholderAvatar(tt.username)

			if !strings.Contains(result, tt.wantChar) {
				t.Errorf("placeholderAvatar(%q) does not contain %q:\n%s", tt.username, tt.wantChar, result)
			}

			if !strings.Contains(result, "+------+") {
				t.Errorf("placeholderAvatar(%q) missing border:\n%s", tt.username, result)
			}

			lines := strings.Split(result, "\n")
			if len(lines) != 5 {
				t.Errorf("placeholderAvatar(%q) has %d lines, want 5:\n%s", tt.username, len(lines), result)
			}
		})
	}
}

func TestImageToHalfBlock(t *testing.T) {
	// Create a 1x1 black image
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.Black)

	result := imageToHalfBlock(img)

	// Should produce avatarHeight/2 rows
	lines := strings.Split(result, "\n")
	expectedRows := avatarHeight / 2
	if len(lines) != expectedRows {
		t.Errorf("imageToHalfBlock 1x1 black: got %d lines, want %d", len(lines), expectedRows)
	}

	// Each line should contain half-block chars (▀) with ANSI escapes
	for _, line := range lines {
		if !strings.Contains(line, "▀") {
			t.Errorf("expected half-block char ▀ in line, got: %q", line)
		}
	}
}

func TestImageToHalfBlockWhite(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)

	result := imageToHalfBlock(img)

	lines := strings.Split(result, "\n")
	expectedRows := avatarHeight / 2
	if len(lines) != expectedRows {
		t.Errorf("imageToHalfBlock 1x1 white: got %d lines, want %d", len(lines), expectedRows)
	}

	// Should contain ANSI color codes for white (255;255;255)
	if !strings.Contains(result, "255;255;255") {
		t.Errorf("expected white color codes (255;255;255) in output")
	}
}

func TestImageToHalfBlockDimensions(t *testing.T) {
	// Create a larger image to test sampling
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.Gray{Y: 128})
		}
	}

	result := imageToHalfBlock(img)

	lines := strings.Split(result, "\n")
	expectedRows := avatarHeight / 2
	if len(lines) != expectedRows {
		t.Errorf("got %d lines, want %d", len(lines), expectedRows)
	}

	// Each line should have exactly avatarWidth half-block characters
	for i, line := range lines {
		count := strings.Count(line, "▀")
		if count != avatarWidth {
			t.Errorf("line %d has %d half-blocks, want %d", i, count, avatarWidth)
		}
	}
}
