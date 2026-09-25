package media

import (
	"fmt"
	"hash/fnv"
)

// GenerateProceduralBustSVG generates a deterministic 3/4 bust silhouette looking right.
func GenerateProceduralBustSVG(id, name, gender string) []byte {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id + ":" + name + ":" + gender))
	seed := h.Sum32()

	hue1 := seed % 360
	hue2 := (hue1 + 40) % 360

	// 3/4 bust looking slightly right: head slightly offset, angled jaw, angled shoulders
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="100%%" height="100%%">
  <defs>
    <linearGradient id="bgGrad" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
      <stop offset="0%%" stop-color="hsl(%d, 35%%, 18%%)" />
      <stop offset="100%%" stop-color="hsl(%d, 40%%, 10%%)" />
    </linearGradient>
    <linearGradient id="bustGrad" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
      <stop offset="0%%" stop-color="hsl(%d, 50%%, 75%%)" />
      <stop offset="100%%" stop-color="hsl(%d, 55%%, 45%%)" />
    </linearGradient>
  </defs>
  <rect width="256" height="256" rx="32" fill="url(#bgGrad)" />
  <!-- Shoulders / Torso angled 3/4 to the right -->
  <path d="M 40 256 C 45 200, 75 170, 115 160 C 130 156, 155 156, 175 165 C 215 180, 235 210, 240 256 Z" fill="url(#bustGrad)" opacity="0.9" />
  <!-- Neck -->
  <path d="M 120 162 L 126 125 L 158 128 L 160 165 Z" fill="url(#bustGrad)" opacity="0.95" />
  <!-- Head 3/4 turned to the right -->
  <ellipse cx="146" cy="95" rx="44" ry="54" fill="url(#bustGrad)" />
  <!-- Jawline contour emphasizing looking right -->
  <path d="M 125 105 Q 145 150 178 125 Q 192 100 188 78 Z" fill="url(#bustGrad)" />
</svg>`, hue1, hue2, hue1, hue2)

	return []byte(svg)
}
