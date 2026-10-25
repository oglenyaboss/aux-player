#ifndef AUX_SINK_WINDOWS_H
#define AUX_SINK_WINDOWS_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* aux_winsink_t;

typedef void (*aux_winsink_eos_cb_t)(uintptr_t user_data);
typedef void (*aux_winsink_err_cb_t)(uintptr_t user_data, char* err_msg);
typedef void (*aux_winsink_stream_start_cb_t)(uintptr_t user_data, char* uri_utf8);

// aux_winsink_create returns a sink that decodes the ADTS AAC stream at a
// loopback URL with Media Foundation and plays the PCM through waveOut.
aux_winsink_t aux_winsink_create(
    uintptr_t user_data,
    aux_winsink_eos_cb_t on_eos,
    aux_winsink_err_cb_t on_err,
    aux_winsink_stream_start_cb_t on_stream_start);
void aux_winsink_play_uri(aux_winsink_t handle, const char* uri_utf8);
void aux_winsink_set_next_uri(aux_winsink_t handle, const char* uri_utf8);
void aux_winsink_play(aux_winsink_t handle);
void aux_winsink_pause(aux_winsink_t handle);
void aux_winsink_stop(aux_winsink_t handle);
void aux_winsink_set_volume(aux_winsink_t handle, double volume);
// aux_winsink_set_eq sets gains in dB for the 10 bands, lowest first.
void aux_winsink_set_eq(aux_winsink_t handle, const double* gains_db, int count);
// aux_winsink_position returns seconds of audio played since the last play_uri.
double aux_winsink_position(aux_winsink_t handle);
void aux_winsink_destroy(aux_winsink_t handle);

#ifdef __cplusplus
}
#endif

#endif // AUX_SINK_WINDOWS_H
