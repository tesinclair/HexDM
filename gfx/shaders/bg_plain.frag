// Just the base colour, a little lighter in the middle.
vec3 scene(vec2 p, vec2 uv) {
	return uBase + uPrimary * 0.14 * exp(-dot(p, p) * 3.0);
}
