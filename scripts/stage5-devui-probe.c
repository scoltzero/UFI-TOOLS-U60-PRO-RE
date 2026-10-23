/* SPDX-License-Identifier: AGPL-3.0-or-later
 * Host-only probe linking the companion's actual data.c/json.c, no display. */
#include "data.h"
#include <stdio.h>
#include <time.h>
#include <unistd.h>

static uint32_t now_ms(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint32_t)(ts.tv_sec * 1000u + ts.tv_nsec / 1000000u);
}
int main(void) {
    devui_data_t initial, next;
    if (!data_backend_init() || !data_refresh(&initial) || !initial.valid ||
        !initial.timezone_available || initial.sim_traffic_n != 1 || initial.rx_bytes != 1000) {
        fputs("initial HTTP state did not reach DevUI parser\n", stderr);
        data_backend_close(); return 1;
    }
    printf("HTTP_STATE_OK timezone=%s sim_count=%d rx_bytes=%ld\n", initial.timezone_label, initial.sim_traffic_n, initial.rx_bytes);
    fflush(stdout);
    uint32_t deadline = now_ms() + 5000;
    while ((int32_t)(deadline - now_ms()) > 0) {
        data_backend_poll(now_ms());
        data_backend_commit_latest();
        if (data_refresh(&next) && next.rx_bytes == 5000) {
            puts("SSE_STATE_OK rx_bytes=5000");
            data_backend_close(); return 0;
        }
        usleep(20000);
    }
    fputs("SSE update did not reach DevUI consumer\n", stderr);
    data_backend_close(); return 1;
}
