#version 300 es
layout(location = 0) in vec2 aPos;   // pixels, origin top left
layout(location = 1) in vec2 aLocal; // pixels from the shape's centre (or atlas uv for text)
layout(location = 2) in vec2 aHalf;  // the shape's half size
layout(location = 3) in vec4 aColor;
layout(location = 4) in vec4 aShape; // kind, corner cut, stroke width, glow radius
layout(location = 5) in vec2 aExtra; // arc start and sweep
uniform vec2 uRes;
out vec2 vLocal;
out vec2 vHalf;
out vec4 vColor;
out vec4 vShape;
out vec2 vExtra;
void main() {
	vLocal = aLocal;
	vHalf = aHalf;
	vColor = aColor;
	vShape = aShape;
	vExtra = aExtra;
	vec2 p = aPos / uRes * 2.0 - 1.0;
	gl_Position = vec4(p.x, -p.y, 0.0, 1.0);
}
