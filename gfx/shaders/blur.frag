#version 300 es
precision highp float;
in vec2 vUv;
uniform sampler2D uScene;
uniform vec2 uDir; // one texel along the blur direction
out vec4 o;
void main() {
	vec3 c = texture(uScene, vUv).rgb * 0.227027;
	c += texture(uScene, vUv + uDir * 1.384615).rgb * 0.316216;
	c += texture(uScene, vUv - uDir * 1.384615).rgb * 0.316216;
	c += texture(uScene, vUv + uDir * 3.230769).rgb * 0.070270;
	c += texture(uScene, vUv - uDir * 3.230769).rgb * 0.070270;
	o = vec4(c, 1.0);
}
