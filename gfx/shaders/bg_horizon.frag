// A lit grid running off to the horizon under a dark sky.
vec3 scene(vec2 p, vec2 uv) {
	const float horizon = -0.19;
	vec3 c = uBase;
	float above = p.y - horizon;

	if (above > 0.0) {
		c += uPrimary * exp(-above * 4.5) * 0.45;
		vec2 q = p * 42.0;
		vec2 id = floor(q);
		float r = hash21(id);
		vec2 at = (vec2(hash21(id + 2.0), hash21(id + 5.0)) - 0.5) * 0.6;
		float d = length(fract(q) - 0.5 - at);
		float twinkle = 0.5 + 0.5 * sin(uTime * 2.0 * r + r * 30.0);
		c += vec3(0.85) * step(0.9, r) * smoothstep(0.07, 0.0, d) * twinkle * smoothstep(0.0, 0.3, above);
	} else {
		float z = 0.12 / max(-above, 0.0005);
		vec2 g = vec2(p.x * z * 7.0, z * 7.0 + uTime * 0.7);
		vec2 w = fwidth(g) * 1.5 + 0.02;
		vec2 f = min(fract(g), 1.0 - fract(g));
		float line = max(1.0 - smoothstep(0.0, w.x, f.x), 1.0 - smoothstep(0.0, w.y, f.y));
		float fade = exp(-z * 0.22);
		c += uAccent * line * fade * (0.6 + 0.6 * uEnergy);
		c += uPrimary * fade * 0.1;
	}

	c += uAccent * exp(-abs(above) * 70.0) * 0.7;
	c += uPrimary * exp(-abs(above) * 12.0) * 0.35;
	return c;
}
