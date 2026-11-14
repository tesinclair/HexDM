#version 300 es
precision highp float;
// Keeps only what is bright enough to glow, at a quarter of the size.
in vec2 vUv;
uniform sampler2D uScene;
uniform vec2 uRes;
out vec4 o;
void main() {
	vec2 px = 1.0 / uRes;
	vec3 c = texture(uScene, vUv + px * vec2(-1.0, -1.0)).rgb
	       + texture(uScene, vUv + px * vec2(1.0, -1.0)).rgb
	       + texture(uScene, vUv + px * vec2(-1.0, 1.0)).rgb
	       + texture(uScene, vUv + px * vec2(1.0, 1.0)).rgb;
	c *= 0.25;
	float l = max(c.r, max(c.g, c.b));
	o = vec4(c * smoothstep(0.35, 0.95, l), 1.0);
}
