// The SDL and OpenGL ES side of the rendered greeter. Everything here must
// be called from the one thread that called hx_init.
#ifndef HX_H
#define HX_H

enum { HX_EV_NONE, HX_EV_QUIT, HX_EV_KEY, HX_EV_TEXT };

enum {
	HX_KEY_OTHER,
	HX_KEY_ENTER,
	HX_KEY_ESC,
	HX_KEY_BACKSPACE,
	HX_KEY_LEFT,
	HX_KEY_RIGHT,
	HX_KEY_UP,
	HX_KEY_DOWN,
	HX_KEY_INTERRUPT,
};

typedef struct {
	int type;
	int key;
	char text[64];
} hx_event;

// Shader programs, in the order a frame uses them.
enum { HX_PROG_BG, HX_PROG_BATCH, HX_PROG_BRIGHT, HX_PROG_BLUR, HX_PROG_POST, HX_PROG_COUNT };

typedef struct {
	int kms;       // draw straight to the screen, never into someone's desktop
	int hidden;    // never show the window (used for snapshots)
	int width;     // window size when there is a desktop to open one on
	int height;
	int secondary; // also cover the other displays with the background
} hx_opts;

typedef struct {
	float time;
	float base[3];
	float primary[3];
	float accent[3];
	float energy;
	float fade;
	float glitch;
	float scanlines;
	float vignette;
	float bloom;
} hx_frame;

// All of these return 1 on success. On failure err holds the reason.
int hx_init(const hx_opts *opts, char *err, int errlen);
int hx_program(int slot, const char *vs, const char *fs, char *err, int errlen);

void hx_shutdown(void);
void hx_size(int *w, int *h);
int hx_poll(hx_event *ev);

// One frame: hx_begin draws the background, hx_batch any number of batches
// of shapes and text on top, hx_end the glow and screen effects.
void hx_atlas(const unsigned char *pixels, int w, int h);
void hx_begin(const hx_frame *frame);
void hx_batch(const float *verts, int nverts);
// If capture is not NULL the finished frame is also written to it as
// width*height RGBA pixels, bottom row first.
void hx_end(int present, unsigned char *capture);

#endif
