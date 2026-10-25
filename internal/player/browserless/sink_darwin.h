#ifndef AUX_SINK_DARWIN_H
#define AUX_SINK_DARWIN_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>

typedef void* aux_darwinsink_t;

typedef void (*aux_darwinsink_eos_cb_t)(uintptr_t user_data);
typedef void (*aux_darwinsink_err_cb_t)(uintptr_t user_data, char* err_msg);
typedef void (*aux_darwinsink_stream_start_cb_t)(uintptr_t user_data, char* uri);

aux_darwinsink_t aux_darwinsink_create(uintptr_t user_data, aux_darwinsink_eos_cb_t on_eos, aux_darwinsink_err_cb_t on_err, aux_darwinsink_stream_start_cb_t on_stream_start);
void aux_darwinsink_play_uri(aux_darwinsink_t handle, const char* uri);
void aux_darwinsink_set_next_uri(aux_darwinsink_t handle, const char* uri);
void aux_darwinsink_play(aux_darwinsink_t handle);
void aux_darwinsink_pause(aux_darwinsink_t handle);
void aux_darwinsink_stop(aux_darwinsink_t handle);
void aux_darwinsink_seek(aux_darwinsink_t handle, double seconds);
void aux_darwinsink_set_volume(aux_darwinsink_t handle, float volume);
void aux_darwinsink_set_eq(aux_darwinsink_t handle, const double* gains_db, int count);
double aux_darwinsink_position(aux_darwinsink_t handle);
void aux_darwinsink_destroy(aux_darwinsink_t handle);

#ifdef __cplusplus
}
#endif

#endif // AUX_SINK_DARWIN_H
