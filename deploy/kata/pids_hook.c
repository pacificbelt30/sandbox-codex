//go:build ignore

/* Derived from agent-orchestrator-platform's MIT-licensed guest prestart hook.
 * Preserve codex-dock's 512-process policy, only on the workload's cgroup. */
#define _GNU_SOURCE
#include <ctype.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static void fail(const char *message) { fprintf(stderr, "codex-dock-pids-hook: %s\n", message); exit(1); }
int main(void) {
    char state[65537], path[4096], line[4096], target[4096];
    size_t n = fread(state, 1, sizeof(state)-1, stdin);
    if (ferror(stdin) || n == sizeof(state)-1) fail("invalid hook state size");
    state[n] = 0;
    char *p = strstr(state, "\"pid\"");
    if (!p || !(p = strchr(p, ':'))) fail("missing hook pid");
    char *end; long pid = strtol(p+1, &end, 10);
    if (pid <= 1 || pid > 2147483647 || end == p+1) fail("invalid hook pid");
    snprintf(path, sizeof(path), "/proc/%ld/cgroup", pid);
    FILE *f = fopen(path, "re"); if (!f) fail("cannot open target cgroup");
    target[0] = 0;
    while (fgets(line, sizeof(line), f)) {
        if (strncmp(line, "0::/", 4)) continue;
        char *cg = line+3; cg[strcspn(cg, "\r\n")] = 0;
        /* The agent/VM root cgroup is never a target. */
        char *last = strrchr(cg, '/');
        if (!last || strlen(last+1) != 64 || strstr(cg, "..")) fail("unsafe cgroup path");
        for (char *q=last+1; *q; q++) if (!isxdigit((unsigned char)*q)) fail("invalid container id");
        if (snprintf(target,sizeof(target),"/sys/fs/cgroup%s/pids.max",cg) >= (int)sizeof(target)) fail("cgroup path too long");
        break;
    }
    fclose(f);
    if (!target[0]) fail("missing unified cgroup");
    int fd = open(target, O_WRONLY|O_CLOEXEC|O_NOFOLLOW);
    if (fd < 0) fail("cannot open pids.max");
    if (write(fd,"512\n",4) != 4) fail("cannot write pids.max");
    close(fd);
    f = fopen(target,"re"); if (!f) fail("cannot verify pids.max");
    if (!fgets(line,sizeof(line),f) || strcmp(line,"512\n")) fail("pids.max readback mismatch");
    fclose(f);
    return 0;
}
