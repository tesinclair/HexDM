// A field of hexagons. Rings of light travel outwards through it.
const vec2 HEX = vec2(1.0, 1.7320508);

float hexDist(vec2 p) {
	p = abs(p);
	return max(dot(p, HEX * 0.5), p.x);
}

// xy: position inside the cell, zw: which cell.
vec4 hexCell(vec2 p) {
	vec4 c = floor(vec4(p, p - vec2(0.5, 1.0)) / HEX.xyxy) + 0.5;
	vec4 h = vec4(p - c.xy * HEX, p - (c.zw + 0.5) * HEX);
	return dot(h.xy, h.xy) < dot(h.zw, h.zw) ? vec4(h.xy, c.xy) : vec4(h.zw, c.zw + 0.5);
}

vec3 scene(vec2 p, vec2 uv) {
	vec2 drift = vec2(uTime * 0.035, uTime * 0.015);
	vec4 h = hexCell(p * 11.0 + drift);
	float edge = 0.5 - hexDist(h.xy);
	float line = smoothstep(0.04, 0.0, edge);
	float inside = smoothstep(0.0, 0.12, edge);
	float rnd = hash21(h.zw);

	vec2 centre = (h.zw * HEX - drift) / 11.0;
	float dist = length(centre);
	float wave = pow(0.5 + 0.5 * sin(dist * 9.0 - uTime * 0.8), 10.0);
	float spark = pow(max(0.0, sin(uTime * (0.4 + rnd) + rnd * 40.0)), 24.0) * step(0.75, rnd);

	vec3 c = uBase;
	c += uPrimary * (0.035 + 0.05 * rnd) * inside;
	c += uPrimary * line * 0.42;
	c += uPrimary * inside * wave * (0.12 + 0.25 * rnd);
	c += uAccent * line * wave * 0.55;
	c += uAccent * inside * spark * 0.22;
	c += uAccent * line * uEnergy * 0.5;
	return c;
}
