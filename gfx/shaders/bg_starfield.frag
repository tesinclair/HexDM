// Drifting stars in four layers over a faint nebula.
vec3 scene(vec2 p, vec2 uv) {
	vec3 c = uBase;

	float n = fbm(p * 1.6 + vec2(uTime * 0.012, 0.0));
	float n2 = fbm(p * 2.7 - vec2(0.0, uTime * 0.015) + n);
	c += uPrimary * pow(n, 2.2) * 0.6;
	c += uAccent * pow(n2, 3.5) * 0.25;

	for (int i = 0; i < 4; i++) {
		float depth = float(i) / 3.0;
		float scale = mix(12.0, 55.0, depth);
		vec2 q = p * scale + vec2(uTime * mix(0.25, 0.12, depth), float(i) * 17.0);
		vec2 id = floor(q);
		float r = hash21(id + float(i) * 7.3);
		if (r > 0.8) {
			vec2 at = (vec2(hash21(id + 1.7), hash21(id + 9.1)) - 0.5) * 0.7;
			float d = length(fract(q) - 0.5 - at);
			float twinkle = 0.6 + 0.4 * sin(uTime * (1.0 + r * 3.0) + r * 50.0);
			float b = smoothstep(0.1, 0.0, d) * twinkle * mix(1.0, 0.4, depth);
			c += mix(vec3(1.0), uAccent, hash21(id + 3.3)) * b * (1.0 + uEnergy);
		}
	}
	return c;
}
