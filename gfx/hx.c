#include "hx.h"

#include <GLES3/gl3.h>
#include <SDL3/SDL.h>
#include <stdio.h>
#include <string.h>

#define FLOATS_PER_VERT 16
#define MAX_EXTRA 7

typedef struct {
	GLuint fbo, tex;
	int w, h;
} target;

static struct {
	SDL_Window *win;
	SDL_GLContext ctx;
	SDL_Window *extra[MAX_EXTRA];
	int nextra;
	int hidden;

	GLuint prog[HX_PROG_COUNT];
	GLuint vao, vbo, atlas;
	GLuint emptyVao;
	size_t vboCap;

	target scene, bloomA, bloomB, final;
	int w, h;
	hx_frame frame;
} g;

static void fail(char *err, int errlen, const char *what, const char *why) {
	if (err && errlen > 0) snprintf(err, errlen, "%s: %s", what, why ? why : "unknown");
}

static void target_free(target *t) {
	if (t->fbo) glDeleteFramebuffers(1, &t->fbo);
	if (t->tex) glDeleteTextures(1, &t->tex);
	memset(t, 0, sizeof *t);
}

static void target_make(target *t, int w, int h) {
	target_free(t);
	t->w = w < 1 ? 1 : w;
	t->h = h < 1 ? 1 : h;
	glGenTextures(1, &t->tex);
	glBindTexture(GL_TEXTURE_2D, t->tex);
	glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA8, t->w, t->h, 0, GL_RGBA, GL_UNSIGNED_BYTE, NULL);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
	glGenFramebuffers(1, &t->fbo);
	glBindFramebuffer(GL_FRAMEBUFFER, t->fbo);
	glFramebufferTexture2D(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_TEXTURE_2D, t->tex, 0);
}

static void target_bind(const target *t) {
	glBindFramebuffer(GL_FRAMEBUFFER, t->fbo);
	glViewport(0, 0, t->w, t->h);
}

// Matches the targets to the window, which can change size at any time.
static void resize(void) {
	int w = 0, h = 0;
	SDL_GetWindowSizeInPixels(g.win, &w, &h);
	if (w < 1) w = 1;
	if (h < 1) h = 1;
	if (w == g.w && h == g.h && g.scene.fbo) return;
	g.w = w;
	g.h = h;
	target_make(&g.scene, w, h);
	target_make(&g.final, w, h);
	target_make(&g.bloomA, w / 4, h / 4);
	target_make(&g.bloomB, w / 4, h / 4);
}

static GLuint compile(GLenum type, const char *src, char *err, int errlen) {
	GLuint s = glCreateShader(type);
	glShaderSource(s, 1, &src, NULL);
	glCompileShader(s);
	GLint ok = 0;
	glGetShaderiv(s, GL_COMPILE_STATUS, &ok);
	if (!ok) {
		char log[1024] = "";
		glGetShaderInfoLog(s, sizeof log, NULL, log);
		fail(err, errlen, type == GL_VERTEX_SHADER ? "vertex shader" : "fragment shader", log);
		glDeleteShader(s);
		return 0;
	}
	return s;
}

int hx_program(int slot, const char *vs, const char *fs, char *err, int errlen) {
	if (slot < 0 || slot >= HX_PROG_COUNT) {
		fail(err, errlen, "program", "no such slot");
		return 0;
	}
	GLuint v = compile(GL_VERTEX_SHADER, vs, err, errlen);
	if (!v) return 0;
	GLuint f = compile(GL_FRAGMENT_SHADER, fs, err, errlen);
	if (!f) {
		glDeleteShader(v);
		return 0;
	}
	GLuint p = glCreateProgram();
	glAttachShader(p, v);
	glAttachShader(p, f);
	glLinkProgram(p);
	glDeleteShader(v);
	glDeleteShader(f);
	GLint ok = 0;
	glGetProgramiv(p, GL_LINK_STATUS, &ok);
	if (!ok) {
		char log[1024] = "";
		glGetProgramInfoLog(p, sizeof log, NULL, log);
		fail(err, errlen, "link", log);
		glDeleteProgram(p);
		return 0;
	}
	if (g.prog[slot]) glDeleteProgram(g.prog[slot]);
	g.prog[slot] = p;
	return 1;
}

// Opens a window on every display but the primary one. Only the background
// is drawn there. Any of them failing is not a reason to stop.
static void open_secondary(void) {
	int n = 0;
	SDL_DisplayID *displays = SDL_GetDisplays(&n);
	SDL_DisplayID primary = SDL_GetDisplayForWindow(g.win);
	for (int i = 0; displays && i < n && g.nextra < MAX_EXTRA; i++) {
		if (displays[i] == primary) continue;
		SDL_PropertiesID props = SDL_CreateProperties();
		SDL_SetStringProperty(props, SDL_PROP_WINDOW_CREATE_TITLE_STRING, "HexDM");
		SDL_SetNumberProperty(props, SDL_PROP_WINDOW_CREATE_X_NUMBER, SDL_WINDOWPOS_UNDEFINED_DISPLAY(displays[i]));
		SDL_SetNumberProperty(props, SDL_PROP_WINDOW_CREATE_Y_NUMBER, SDL_WINDOWPOS_UNDEFINED_DISPLAY(displays[i]));
		SDL_SetBooleanProperty(props, SDL_PROP_WINDOW_CREATE_OPENGL_BOOLEAN, true);
		SDL_SetBooleanProperty(props, SDL_PROP_WINDOW_CREATE_FULLSCREEN_BOOLEAN, true);
		SDL_Window *w = SDL_CreateWindowWithProperties(props);
		SDL_DestroyProperties(props);
		if (!w) {
			SDL_Log("hexdm: no window on display %u: %s", (unsigned)displays[i], SDL_GetError());
			continue;
		}
		g.extra[g.nextra++] = w;
	}
	SDL_free(displays);
}

int hx_init(const hx_opts *opts, char *err, int errlen) {
	memset(&g, 0, sizeof g);
	g.hidden = opts->hidden;

	// Go owns the signals. SDL's handlers would also run on a stack Go
	// does not expect.
	SDL_SetHint(SDL_HINT_NO_SIGNAL_HANDLERS, "1");
	SDL_SetAppMetadata("HexDM", NULL, "hexdm");
	if (opts->kms) SDL_SetHint(SDL_HINT_VIDEO_DRIVER, "kmsdrm");

	if (!SDL_Init(SDL_INIT_VIDEO)) {
		fail(err, errlen, "SDL_Init", SDL_GetError());
		return 0;
	}

	SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK, SDL_GL_CONTEXT_PROFILE_ES);
	SDL_GL_SetAttribute(SDL_GL_CONTEXT_MAJOR_VERSION, 3);
	SDL_GL_SetAttribute(SDL_GL_CONTEXT_MINOR_VERSION, 0);
	SDL_GL_SetAttribute(SDL_GL_DOUBLEBUFFER, 1);
	SDL_GL_SetAttribute(SDL_GL_DEPTH_SIZE, 0);
	SDL_GL_SetAttribute(SDL_GL_SHARE_WITH_CURRENT_CONTEXT, 1);

	SDL_WindowFlags flags = SDL_WINDOW_OPENGL | SDL_WINDOW_HIGH_PIXEL_DENSITY;
	if (opts->kms) flags |= SDL_WINDOW_FULLSCREEN;
	else flags |= SDL_WINDOW_RESIZABLE;
	if (opts->hidden) flags |= SDL_WINDOW_HIDDEN;

	g.win = SDL_CreateWindow("HexDM", opts->width, opts->height, flags);
	if (!g.win) {
		fail(err, errlen, "SDL_CreateWindow", SDL_GetError());
		SDL_Quit();
		return 0;
	}
	g.ctx = SDL_GL_CreateContext(g.win);
	if (!g.ctx) {
		fail(err, errlen, "SDL_GL_CreateContext", SDL_GetError());
		SDL_DestroyWindow(g.win);
		SDL_Quit();
		return 0;
	}

	if (opts->kms && opts->secondary) open_secondary();
	SDL_GL_MakeCurrent(g.win, g.ctx);
	SDL_GL_SetSwapInterval(1);
	SDL_HideCursor();
	SDL_StartTextInput(g.win);

	glGenVertexArrays(1, &g.emptyVao);
	glGenVertexArrays(1, &g.vao);
	glGenBuffers(1, &g.vbo);
	glBindVertexArray(g.vao);
	glBindBuffer(GL_ARRAY_BUFFER, g.vbo);
	// position, local, half size, colour, shape, extra
	static const int sizes[] = {2, 2, 2, 4, 4, 2};
	int offset = 0;
	for (int i = 0; i < 6; i++) {
		glEnableVertexAttribArray(i);
		glVertexAttribPointer(i, sizes[i], GL_FLOAT, GL_FALSE, FLOATS_PER_VERT * sizeof(float),
		                      (void *)(offset * sizeof(float)));
		offset += sizes[i];
	}
	glBindVertexArray(0);

	glGenTextures(1, &g.atlas);
	glBindTexture(GL_TEXTURE_2D, g.atlas);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);

	glDisable(GL_DEPTH_TEST);
	glDisable(GL_CULL_FACE);
	resize();
	return 1;
}

void hx_shutdown(void) {
	if (g.ctx) {
		target_free(&g.scene);
		target_free(&g.final);
		target_free(&g.bloomA);
		target_free(&g.bloomB);
		SDL_GL_DestroyContext(g.ctx);
	}
	for (int i = 0; i < g.nextra; i++) SDL_DestroyWindow(g.extra[i]);
	if (g.win) SDL_DestroyWindow(g.win);
	SDL_Quit();
	memset(&g, 0, sizeof g);
}

void hx_size(int *w, int *h) {
	resize();
	*w = g.w;
	*h = g.h;
}

int hx_poll(hx_event *ev) {
	SDL_Event e;
	while (SDL_PollEvent(&e)) {
		memset(ev, 0, sizeof *ev);
		switch (e.type) {
		case SDL_EVENT_QUIT:
		case SDL_EVENT_WINDOW_CLOSE_REQUESTED:
			ev->type = HX_EV_QUIT;
			return 1;
		case SDL_EVENT_TEXT_INPUT:
			if (!e.text.text || !e.text.text[0]) break;
			ev->type = HX_EV_TEXT;
			SDL_strlcpy(ev->text, e.text.text, sizeof ev->text);
			return 1;
		case SDL_EVENT_KEY_DOWN:
			ev->type = HX_EV_KEY;
			switch (e.key.key) {
			case SDLK_RETURN:
			case SDLK_KP_ENTER: ev->key = HX_KEY_ENTER; break;
			case SDLK_ESCAPE: ev->key = HX_KEY_ESC; break;
			case SDLK_BACKSPACE: ev->key = HX_KEY_BACKSPACE; break;
			case SDLK_LEFT: ev->key = HX_KEY_LEFT; break;
			case SDLK_RIGHT: ev->key = HX_KEY_RIGHT; break;
			case SDLK_UP: ev->key = HX_KEY_UP; break;
			case SDLK_DOWN: ev->key = HX_KEY_DOWN; break;
			case SDLK_C:
				if (e.key.mod & SDL_KMOD_CTRL) ev->key = HX_KEY_INTERRUPT;
				break;
			default: break;
			}
			// A held enter must not log in (or shut down) twice.
			if (e.key.repeat && ev->key == HX_KEY_ENTER) ev->key = HX_KEY_OTHER;
			if (ev->key != HX_KEY_OTHER) return 1;
			break;
		default: break;
		}
	}
	// Typed characters pass through SDL's event queue. Nothing of them
	// is kept here.
	memset(&e, 0, sizeof e);
	return 0;
}

void hx_atlas(const unsigned char *pixels, int w, int h) {
	glBindTexture(GL_TEXTURE_2D, g.atlas);
	glPixelStorei(GL_UNPACK_ALIGNMENT, 1);
	glTexImage2D(GL_TEXTURE_2D, 0, GL_R8, w, h, 0, GL_RED, GL_UNSIGNED_BYTE, pixels);
}

static void use(int slot) {
	GLuint p = g.prog[slot];
	glUseProgram(p);
	glUniform2f(glGetUniformLocation(p, "uRes"), (float)g.w, (float)g.h);
	glUniform1f(glGetUniformLocation(p, "uTime"), g.frame.time);
}

static void fullscreen(void) {
	glBindVertexArray(g.emptyVao);
	glDrawArrays(GL_TRIANGLES, 0, 3);
}

static void background(float w, float h, float fade) {
	GLuint p = g.prog[HX_PROG_BG];
	use(HX_PROG_BG);
	glUniform2f(glGetUniformLocation(p, "uRes"), w, h);
	glUniform3fv(glGetUniformLocation(p, "uBase"), 1, g.frame.base);
	glUniform3fv(glGetUniformLocation(p, "uPrimary"), 1, g.frame.primary);
	glUniform3fv(glGetUniformLocation(p, "uAccent"), 1, g.frame.accent);
	glUniform1f(glGetUniformLocation(p, "uEnergy"), g.frame.energy);
	glUniform1f(glGetUniformLocation(p, "uFade"), fade);
	glDisable(GL_BLEND);
	fullscreen();
}

void hx_begin(const hx_frame *frame) {
	g.frame = *frame;
	resize();
	target_bind(&g.scene);
	background((float)g.w, (float)g.h, 1.0f);
}

void hx_batch(const float *verts, int nverts) {
	if (nverts <= 0) return;
	size_t bytes = (size_t)nverts * FLOATS_PER_VERT * sizeof(float);
	use(HX_PROG_BATCH);
	glUniform1i(glGetUniformLocation(g.prog[HX_PROG_BATCH], "uAtlas"), 0);
	glActiveTexture(GL_TEXTURE0);
	glBindTexture(GL_TEXTURE_2D, g.atlas);
	glBindVertexArray(g.vao);
	glBindBuffer(GL_ARRAY_BUFFER, g.vbo);
	if (bytes > g.vboCap) {
		glBufferData(GL_ARRAY_BUFFER, bytes, verts, GL_STREAM_DRAW);
		g.vboCap = bytes;
	} else {
		glBufferSubData(GL_ARRAY_BUFFER, 0, bytes, verts);
	}
	// Colours are premultiplied, so glow can add light without covering.
	glEnable(GL_BLEND);
	glBlendFunc(GL_ONE, GL_ONE_MINUS_SRC_ALPHA);
	glDrawArrays(GL_TRIANGLES, 0, nverts);
	glDisable(GL_BLEND);
}

static void blur(const target *from, const target *to, float dx, float dy) {
	target_bind(to);
	use(HX_PROG_BLUR);
	glUniform2f(glGetUniformLocation(g.prog[HX_PROG_BLUR], "uDir"), dx / from->w, dy / from->h);
	glBindTexture(GL_TEXTURE_2D, from->tex);
	fullscreen();
}

static void post(float fade) {
	GLuint p = g.prog[HX_PROG_POST];
	use(HX_PROG_POST);
	glUniform1i(glGetUniformLocation(p, "uScene"), 0);
	glUniform1i(glGetUniformLocation(p, "uBloomTex"), 1);
	glUniform1f(glGetUniformLocation(p, "uFade"), fade);
	glUniform1f(glGetUniformLocation(p, "uGlitch"), g.frame.glitch);
	glUniform1f(glGetUniformLocation(p, "uScan"), g.frame.scanlines);
	glUniform1f(glGetUniformLocation(p, "uVignette"), g.frame.vignette);
	glUniform1f(glGetUniformLocation(p, "uBloom"), g.frame.bloom);
	glActiveTexture(GL_TEXTURE1);
	glBindTexture(GL_TEXTURE_2D, g.bloomA.tex);
	glActiveTexture(GL_TEXTURE0);
	glBindTexture(GL_TEXTURE_2D, g.scene.tex);
	fullscreen();
}

void hx_end(int present, unsigned char *capture) {
	glDisable(GL_BLEND);
	glActiveTexture(GL_TEXTURE0);

	if (g.frame.bloom > 0.0f) {
		target_bind(&g.bloomA);
		use(HX_PROG_BRIGHT);
		glBindTexture(GL_TEXTURE_2D, g.scene.tex);
		fullscreen();
		blur(&g.bloomA, &g.bloomB, 1.0f, 0.0f);
		blur(&g.bloomB, &g.bloomA, 0.0f, 1.0f);
		blur(&g.bloomA, &g.bloomB, 2.0f, 0.0f);
		blur(&g.bloomB, &g.bloomA, 0.0f, 2.0f);
	}

	if (capture) {
		target_bind(&g.final);
		post(g.frame.fade);
		glPixelStorei(GL_PACK_ALIGNMENT, 1);
		glReadPixels(0, 0, g.w, g.h, GL_RGBA, GL_UNSIGNED_BYTE, capture);
	}

	if (!present) {
		glFlush();
		return;
	}

	glBindFramebuffer(GL_FRAMEBUFFER, 0);
	glViewport(0, 0, g.w, g.h);
	post(g.frame.fade);
	SDL_GL_SwapWindow(g.win);

	// The other displays get the background on its own.
	for (int i = 0; i < g.nextra; i++) {
		int w = 0, h = 0;
		if (!SDL_GL_MakeCurrent(g.extra[i], g.ctx)) continue;
		SDL_GetWindowSizeInPixels(g.extra[i], &w, &h);
		glBindFramebuffer(GL_FRAMEBUFFER, 0);
		glViewport(0, 0, w, h);
		background((float)w, (float)h, g.frame.fade);
		SDL_GL_SwapWindow(g.extra[i]);
	}
	if (g.nextra) SDL_GL_MakeCurrent(g.win, g.ctx);
}
