#version 300 es
precision highp float;
in vec2 vLocal;
in vec2 vHalf;
in vec4 vColor;
in vec4 vShape;
in vec2 vExtra;
uniform sampler2D uAtlas;
out vec4 o;

// A box with its corners cut off at 45 degrees.
float sdBox(vec2 p, vec2 b, float cut) {
	vec2 q = abs(p) - b;
	float d = length(max(q, 0.0)) + min(max(q.x, q.y), 0.0);
	float c = (abs(p.x) + abs(p.y) - (b.x + b.y - cut)) * 0.70710678;
	return max(d, c);
}

// A hexagon, point up. r is the distance from the centre to a flat side.
float sdHex(vec2 p, float r) {
	const vec3 k = vec3(-0.866025404, 0.5, 0.577350269);
	p = abs(p.yx);
	p -= 2.0 * min(dot(k.xy, p), 0.0) * k.xy;
	p -= vec2(clamp(p.x, -k.z * r, k.z * r), r);
	return length(p) * sign(p.y);
}

void main() {
	int kind = int(vShape.x + 0.5);
	if (kind == 0) {
		float a = texture(uAtlas, vLocal).r * vColor.a;
		o = vec4(vColor.rgb * a, a);
		return;
	}

	float d;
	if (kind == 1) d = sdBox(vLocal, vHalf, vShape.y);
	else if (kind == 2) d = length(vLocal) - vHalf.x;
	else d = sdHex(vLocal, vHalf.x);

	// A stroke lies just inside the edge. Zero width means filled.
	float stroke = vShape.z;
	float e = stroke > 0.0 ? abs(d + stroke * 0.5) - stroke * 0.5 : d;
	float shape = 1.0 - smoothstep(-0.75, 0.75, e);

	float mask = 1.0;
	if (kind == 2 && vExtra.y < 6.2831) {
		// Only part of the ring: an arc from start, sweeping clockwise.
		float r = length(vLocal);
		float rel = mod(atan(vLocal.y, vLocal.x) - vExtra.x, 6.2831853);
		mask = smoothstep(-0.75, 0.75, min(rel, vExtra.y - rel) * r);
	}

	float glow = vShape.w > 0.0 ? exp(-max(e, 0.0) / vShape.w) * 0.5 : 0.0;
	float a = vColor.a * mask;
	// Premultiplied: the glow adds light but covers nothing.
	o = vec4(vColor.rgb * a * (shape + glow * (1.0 - shape)), a * shape);
}
