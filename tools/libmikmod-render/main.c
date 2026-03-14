#include <mikmod.h>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>

static void usage(const char *argv0) {
    fprintf(stderr, "Usage: %s -input module -output file.wav -frames N\n", argv0);
}

static long file_size(const char *path) {
    struct stat st;

    if (stat(path, &st) != 0) {
        return -1;
    }
    return (long)st.st_size;
}

int main(int argc, char **argv) {
    const char *input = NULL;
    const char *output = NULL;
    long frames = 0;
    char cmdline[4096];
    MODULE *module = NULL;
    long target_bytes;

    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "-input") == 0 && i + 1 < argc) {
            input = argv[++i];
        } else if (strcmp(argv[i], "-output") == 0 && i + 1 < argc) {
            output = argv[++i];
        } else if (strcmp(argv[i], "-frames") == 0 && i + 1 < argc) {
            frames = strtol(argv[++i], NULL, 10);
        } else {
            usage(argv[0]);
            return 1;
        }
    }

    if (input == NULL || output == NULL || frames <= 0) {
        usage(argv[0]);
        return 1;
    }

    MikMod_InitThreads();
    MikMod_RegisterDriver(&drv_wav);
    MikMod_RegisterAllLoaders();

    md_mode |= DMODE_SOFT_MUSIC | DMODE_16BITS | DMODE_STEREO | DMODE_INTERP;
    md_mixfreq = 44100;

    if (snprintf(cmdline, sizeof(cmdline), "file=%s", output) >= (int)sizeof(cmdline)) {
        fprintf(stderr, "Output path too long\n");
        return 1;
    }

    if (MikMod_Init(cmdline)) {
        fprintf(stderr, "MikMod_Init failed: %s\n", MikMod_strerror(MikMod_errno));
        return 1;
    }

    module = Player_Load(input, 64, 0);
    if (module == NULL) {
        fprintf(stderr, "Player_Load failed: %s\n", MikMod_strerror(MikMod_errno));
        MikMod_Exit();
        return 1;
    }

    module->loop = 0;
    Player_Start(module);

    target_bytes = 44 + frames * 4;
    while (Player_Active()) {
        MikMod_Update();
        if (file_size(output) >= target_bytes) {
            break;
        }
    }

    Player_Stop();
    Player_Free(module);
    MikMod_Exit();

    return 0;
}
