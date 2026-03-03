package profile

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/henry/randomchat/internal/config"
)

const (
	avatarWidth  = 20
	avatarHeight = 20 // must be even; rendered as avatarHeight/2 rows
)

func FetchAvatar(cfg *config.Config, avatarURL, username string) (string, error) {
	if err := os.MkdirAll(cfg.AvatarCacheDir, 0700); err != nil {
		return "", err
	}

	// Sanitize username to prevent path traversal
	safeUsername := filepath.Base(username)
	if safeUsername == "." || safeUsername == "/" || safeUsername == string(filepath.Separator) {
		safeUsername = "_invalid_"
	}
	cachePath := filepath.Join(cfg.AvatarCacheDir, safeUsername+".halfblock.txt")
	if data, err := os.ReadFile(cachePath); err == nil {
		return string(data), nil
	}

	// Clear old ASCII cache
	oldCache := filepath.Join(cfg.AvatarCacheDir, safeUsername+".txt")
	os.Remove(oldCache)

	if avatarURL == "" {
		return placeholderAvatar(username), nil
	}

	sep := "?"
	if strings.Contains(avatarURL, "?") {
		sep = "&"
	}
	url := avatarURL + sep + "s=128"

	resp, err := http.Get(url)
	if err != nil {
		return placeholderAvatar(username), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return placeholderAvatar(username), nil
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return placeholderAvatar(username), nil
	}

	result := imageToHalfBlock(img)

	os.WriteFile(cachePath, []byte(result), 0600)

	return result, nil
}

func imageToHalfBlock(img image.Image) string {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	var sb strings.Builder
	for y := 0; y < avatarHeight; y += 2 {
		for x := 0; x < avatarWidth; x++ {
			srcX := bounds.Min.X + (x*srcW)/avatarWidth
			srcYTop := bounds.Min.Y + (y*srcH)/avatarHeight
			srcYBot := bounds.Min.Y + ((y+1)*srcH)/avatarHeight

			rt, gt, bt, _ := img.At(srcX, srcYTop).RGBA()
			rb, gb, bb, _ := img.At(srcX, srcYBot).RGBA()

			// RGBA returns 0-65535, convert to 0-255
			r1, g1, b1 := rt>>8, gt>>8, bt>>8
			r2, g2, b2 := rb>>8, gb>>8, bb>>8

			// ▀ with fg=top pixel, bg=bottom pixel
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀\x1b[0m",
				r1, g1, b1, r2, g2, b2)
		}
		if y+2 < avatarHeight {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func placeholderAvatar(username string) string {
	ch := "?"
	if len(username) > 0 {
		ch = string([]rune(username)[0])
	}
	return fmt.Sprintf(`+------+
|      |
|  %s   |
|      |
+------+`, ch)
}
