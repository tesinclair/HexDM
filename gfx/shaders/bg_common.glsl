#version 300 es
precision highp float;
// Shared by every background. A background supplies:
//   vec3 scene(vec2 p, vec2 uv)
// p is centred with the screen one unit tall and y up; uv runs 0 to 1.
in vec2 vUv;
uniform vec2 uRes;
uniform float uTime;
uniform vec3 uBase;
uniform vec3 uPrimary;
uniform vec3 uAccent;
uniform float uEnergy; // rises briefly when something happens
uniform float uFade;
out vec4 o;

float hash11(float n) { return fract(sin(n * 127.1) * 43758.5453); }
float hash21(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }

float noise(vec2 p) {
	vec2 i = floor(p);
	vec2 f = fract(p);
	f = f * f * (3.0 - 2.0 * f);
	return mix(mix(hash21(i), hash21(i + vec2(1.0, 0.0)), f.x),
	           mix(hash21(i + vec2(0.0, 1.0)), hash21(i + vec2(1.0, 1.0)), f.x), f.y);
}

float fbm(vec2 p) {
	float v = 0.0;
	float a = 0.5;
	for (int i = 0; i < 5; i++) {
		v += a * noise(p);
		p = p * 2.03 + vec2(13.7, 7.1);
		a *= 0.5;
	}
	return v;
}

vec3 scene(vec2 p, vec2 uv);

void main() {
	vec2 p = (gl_FragCoord.xy - 0.5 * uRes) / uRes.y;
	vec3 c = scene(p, gl_FragCoord.xy / uRes);
	// Quieter behind the login column, so it stays easy to read.
	float calm = mix(0.5, 1.0, smoothstep(0.12, 0.62, length(p * vec2(1.5, 0.85))));
	c = mix(uBase, c, calm);
	// A little noise hides the banding in dark gradients.
	c += (hash21(gl_FragCoord.xy + fract(uTime)) - 0.5) / 255.0;
	o = vec4(max(c, 0.0) * uFade, 1.0);
}
