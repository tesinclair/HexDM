// Falling columns of data.
vec3 scene(vec2 p, vec2 uv) {
	const float columns = 84.0; // per screen height
	const float rows = 46.0;
	vec3 c = uBase + uPrimary * 0.04;

	float x = p.x * columns;
	float y = (1.0 - uv.y) * rows;
	float col = floor(x);
	float r = hash11(col);
	float on = step(0.4, hash11(col * 1.7 + 3.0));

	float head = fract(r * 13.0 + uTime * (0.06 + r * 0.16)) * 1.7;
	float behind = head - floor(y) / rows;
	float trail = behind > 0.0 ? exp(-behind * 5.0) : 0.0;
	float flicker = 0.55 + 0.45 * hash21(vec2(col, floor(y) + floor(uTime * (3.0 + 6.0 * r))));

	vec2 f = fract(vec2(x, y));
	float cell = step(0.18, f.x) * step(f.x, 0.82) * step(0.22, f.y) * step(f.y, 0.78);

	c += uPrimary * trail * flicker * cell * on * 0.7;
	c += uAccent * smoothstep(0.035, 0.0, abs(behind)) * cell * on * (0.9 + uEnergy);
	return c;
}
