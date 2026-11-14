#version 300 es
precision highp float;
// The last pass: glow, colour fringing, scanlines, vignette and fades.
in vec2 vUv;
uniform sampler2D uScene;
uniform sampler2D uBloomTex;
uniform vec2 uRes;
uniform float uTime;
uniform float uFade;
uniform float uGlitch;
uniform float uScan;
uniform float uVignette;
uniform float uBloom;
out vec4 o;

float hash(float n) { return fract(sin(n) * 43758.5453); }

void main() {
	vec2 uv = vUv;
	if (uGlitch > 0.0) {
		// Tear a few rows sideways.
		float row = floor(uv.y * 36.0);
		float beat = floor(uTime * 24.0);
		uv.x += (hash(row * 12.9 + beat * 7.1) - 0.5) * 0.05 * uGlitch * step(0.7, hash(row + beat));
	}

	// Fringing grows towards the edges, so the middle stays sharp.
	vec2 fromCentre = vUv - 0.5;
	vec2 split = fromCentre * dot(fromCentre, fromCentre) * 0.003 + vec2(uGlitch * 0.006, 0.0);
	vec3 c = vec3(texture(uScene, uv + split).r, texture(uScene, uv).g, texture(uScene, uv - split).b);

	c += texture(uBloomTex, uv).rgb * uBloom * 0.85;

	float lines = max(1.0, floor(uRes.y / 1080.0 + 0.5));
	c *= 1.0 - uScan * 0.35 * (0.5 + 0.5 * sin(gl_FragCoord.y * 2.0944 / lines));

	float v = length(fromCentre * vec2(1.0, 0.9));
	c *= mix(1.0, smoothstep(0.95, 0.25, v), clamp(uVignette, 0.0, 1.0) * 0.8);

	o = vec4(c * uFade, 1.0);
}
