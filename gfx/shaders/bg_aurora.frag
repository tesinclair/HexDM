// Slow curtains of light.
vec3 scene(vec2 p, vec2 uv) {
	vec3 c = uBase;
	float t = uTime * 0.05;
	for (int i = 0; i < 3; i++) {
		float fi = float(i);
		float sway = (fbm(vec2(p.x * 1.2 + fi * 3.1 + t, t * 0.7 + fi)) - 0.5) * 0.9;
		float y = p.y * 1.4 + 0.2 - fi * 0.2 + sway;
		float band = exp(-y * y * 14.0);
		float rays = 0.55 + 0.45 * fbm(vec2(p.x * 9.0 + fi * 5.0 - t * 2.0, p.y * 1.5 + t));
		c += mix(uPrimary, uAccent, 0.1 + fi * 0.3) * band * rays * (0.34 + 0.1 * uEnergy);
	}
	return c;
}
