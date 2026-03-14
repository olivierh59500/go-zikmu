#include <mikmod.h>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static void usage(const char *argv0) {
    fprintf(stderr, "Usage: %s -input module -frames N [-channel C]... [-all-voices]\n", argv0);
}

static long parse_long(const char *value, const char *label) {
    char *end = NULL;
    long parsed = strtol(value, &end, 10);
    if (end == value || *end != '\0') {
        fprintf(stderr, "Invalid %s: %s\n", label, value);
        exit(1);
    }
    return parsed;
}

static int render_frames(long frames) {
    unsigned char buffer[4096];
    long remaining_bytes = frames * 4;

    while (remaining_bytes > 0 && Player_Active()) {
        ULONG want = (ULONG)((remaining_bytes < (long)sizeof(buffer)) ? remaining_bytes : (long)sizeof(buffer));
        ULONG got = VC_WriteBytes((SBYTE *)buffer, want);
        if (got == 0) {
            break;
        }
        remaining_bytes -= (long)got;
    }

    return remaining_bytes == 0 ? 0 : 1;
}

int main(int argc, char **argv) {
    const char *input = NULL;
    long frames = -1;
    int dump_all = 0;
    int channels[128];
    int channel_count = 0;
    MODULE *module = NULL;
    VOICEINFO voices[256];
    int voice_count = 0;

    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "-input") == 0 && i + 1 < argc) {
            input = argv[++i];
        } else if (strcmp(argv[i], "-frames") == 0 && i + 1 < argc) {
            frames = parse_long(argv[++i], "frames");
        } else if (strcmp(argv[i], "-channel") == 0 && i + 1 < argc) {
            if (channel_count >= (int)(sizeof(channels) / sizeof(channels[0]))) {
                fprintf(stderr, "Too many channels requested\n");
                return 1;
            }
            channels[channel_count++] = (int)parse_long(argv[++i], "channel");
        } else if (strcmp(argv[i], "-all-voices") == 0) {
            dump_all = 1;
        } else {
            usage(argv[0]);
            return 1;
        }
    }

    if (input == NULL || frames < 0) {
        usage(argv[0]);
        return 1;
    }

    MikMod_InitThreads();
    MikMod_RegisterDriver(&drv_nos);
    MikMod_RegisterAllLoaders();

    md_mode |= DMODE_SOFT_MUSIC | DMODE_16BITS | DMODE_STEREO | DMODE_INTERP;
    md_mixfreq = 44100;

    if (MikMod_Init("")) {
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

    if (render_frames(frames) != 0) {
        fprintf(stderr, "Failed to render %ld frames\n", frames);
        Player_Stop();
        Player_Free(module);
        MikMod_Exit();
        return 1;
    }

    voice_count = Player_QueryVoices((UWORD)(sizeof(voices) / sizeof(voices[0])), voices);
    printf("frames=%ld order=%d row=%d active=%d voices=%d\n", frames, Player_GetOrder(), Player_GetRow(), Player_Active(), voice_count);

    for (int i = 0; i < channel_count; i++) {
        int ch = channels[i];
        int voice = Player_GetChannelVoice((UBYTE)ch);
        long pos = -1;
        unsigned long mix_frequency = 0;
        unsigned long mix_panning = 0;
        unsigned int mix_volume = 0;
        if (voice >= 0) {
            pos = Voice_GetPosition((SBYTE)voice);
            mix_frequency = Voice_GetFrequency((SBYTE)voice);
            mix_panning = Voice_GetPanning((SBYTE)voice);
            mix_volume = Voice_GetVolume((SBYTE)voice);
        }
        printf("channel=%d voice=%d position=%ld\n", ch, voice, pos);
        if (voice >= 0 && voice < voice_count) {
            printf("voice=%d kick=%u channel_volume=%d channel_panning=%d period=%u mix_volume=%u mix_panning=%lu mix_frequency=%lu instrument=%p sample=%p\n",
                   voice,
                   voices[voice].kick,
                   voices[voice].volume,
                   voices[voice].panning,
                   voices[voice].period,
                   mix_volume,
                   mix_panning,
                   mix_frequency,
                   (void *)voices[voice].i,
                   (void *)voices[voice].s);
        }
    }

    if (dump_all) {
        for (int voice = 0; voice < voice_count; voice++) {
            ULONG mix_volume = Voice_GetVolume((SBYTE)voice);
            ULONG mix_panning = Voice_GetPanning((SBYTE)voice);
            ULONG mix_frequency = Voice_GetFrequency((SBYTE)voice);
            SLONG position = Voice_GetPosition((SBYTE)voice);
            if (voices[voice].s == NULL && mix_volume == 0) {
                continue;
            }
            printf("active_voice=%d kick=%u channel_volume=%d channel_panning=%d period=%u mix_volume=%u mix_panning=%u mix_frequency=%u position=%d instrument=%p sample=%p\n",
                   voice,
                   voices[voice].kick,
                   voices[voice].volume,
                   voices[voice].panning,
                   voices[voice].period,
                   mix_volume,
                   mix_panning,
                   mix_frequency,
                   position,
                   (void *)voices[voice].i,
                   (void *)voices[voice].s);
        }
    }

    Player_Stop();
    Player_Free(module);
    MikMod_Exit();
    return 0;
}
