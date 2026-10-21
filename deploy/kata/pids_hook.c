//go:build ignore

/* Derived from agent-orchestrator-platform's MIT-licensed guest prestart hook.
 * Apply the node-selected PID policy only to the workload's cgroup.
 * Legacy nodes without the trusted OCI annotation retain 512 processes. */
#define _GNU_SOURCE
#include <ctype.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static void fail(const char *message) { fprintf(stderr, "codex-dock-pids-hook: %s\n", message); exit(1); }

/* Hook state comes from the pinned Kata agent, not a worker request. Find an
 * exact object member while skipping quoted strings and nested values. Never
 * log the state: unrelated annotations may contain private host metadata. */
static const char *member(const char *object, const char *key) {
    if (!object || *object != '{') fail("invalid hook object");
    int depth = 1;
    const char *found = NULL;
    size_t keylen = strlen(key);
    for (const char *p = object + 1; *p; p++) {
        if (*p == '"') {
            const char *start = ++p;
            while (*p && *p != '"') {
                if ((unsigned char)*p < 32) fail("invalid hook string");
                if (*p == '\\') { if (!*++p) fail("truncated hook string"); }
                p++;
            }
            if (!*p) fail("truncated hook string");
            const char *next = p + 1;
            while (isspace((unsigned char)*next)) next++;
            if (depth == 1 && *next == ':' && (size_t)(p-start) == keylen && !memcmp(start,key,keylen)) {
                if (found) fail("duplicate hook policy member");
                next++;
                while (isspace((unsigned char)*next)) next++;
                found = next;
            }
        } else if (*p == '{' || *p == '[') {
            if (++depth > 128) fail("hook state nesting limit");
        } else if (*p == '}' || *p == ']') {
            if (--depth == 0) return found;
        }
    }
    fail("truncated hook object");
    return NULL;
}
static long requested_limit(const char *state) {
    const char *annotations = member(state,"annotations");
    if (!annotations) return 512;
    const char *p = member(annotations,"io.codex-dock.pids-limit");
    if (!p) return 512;
    if (*p++ != '"') fail("PID policy must be a decimal string");
    long limit = 0;
    int digits = 0;
    while (*p >= '0' && *p <= '9') {
        if (++digits > 4) fail("PID policy outside supported range");
        limit = limit*10 + (*p++-'0');
    }
    if (!digits || *p != '"' || limit < 64 || limit > 4096) fail("invalid PID policy");
    return limit;
}

int main(void) {
    char state[65537], path[4096], line[4096], target[4096];
    size_t n = fread(state, 1, sizeof(state)-1, stdin);
    if (ferror(stdin) || n == sizeof(state)-1) fail("invalid hook state size");
    state[n] = 0;
    const char *p = member(state,"pid");
    if (!p) fail("missing hook pid");
    char *end; long pid = strtol(p, &end, 10);
    if (pid <= 1 || pid > 2147483647 || end == p) fail("invalid hook pid");
    long limit = requested_limit(state);
    char value[32]; int value_size = snprintf(value,sizeof(value),"%ld\n",limit);
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
    if (write(fd,value,(size_t)value_size) != value_size) fail("cannot write pids.max");
    close(fd);
    f = fopen(target,"re"); if (!f) fail("cannot verify pids.max");
    if (!fgets(line,sizeof(line),f) || strcmp(line,value)) fail("pids.max readback mismatch");
    fclose(f);
    return 0;
}
