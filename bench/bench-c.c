/* The same three workloads as bench.brick, written plainly in C. */
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static int64_t part_int(int64_t n)
{
    int64_t x = 0;
    int64_t i = 0;

    while (i < n) {
        x = (x * 31 + i) % 1000000007;
        i++;
    }
    return x;
}

static int64_t part_sieve(int64_t n)
{
    unsigned char *flags = malloc((size_t)n);
    int64_t        count = 0;
    int64_t        i, j, k;

    for (i = 0; i < n; i++) {
        flags[i] = 1;
    }
    for (j = 2; j < n; j++) {
        if (flags[j]) {
            count++;
            for (k = j * 2; k < n; k += j) {
                flags[k] = 0;
            }
        }
    }
    free(flags);
    return count;
}

static int64_t part_text(int64_t n)
{
    size_t cap = 64;
    size_t len = 0;
    char  *buf = malloc(cap);
    char   tmp[32];
    int64_t i;
    int64_t result;

    for (i = 0; i < n; i++) {
        int k = snprintf(tmp, sizeof tmp, "%lld,", (long long)i);
        if (len + (size_t)k + 1 > cap) {
            cap = cap * 2 + (size_t)k;
            buf = realloc(buf, cap);
        }
        memcpy(buf + len, tmp, (size_t)k);
        len += (size_t)k;
    }
    buf[len] = '\0';
    result = (int64_t)strlen(buf);
    free(buf);
    return result;
}

int main(int argc, char **argv)
{
    if (argc < 2) {
        fprintf(stderr, "usage: bench int|sieve|text\n");
        return 1;
    }
    if (strcmp(argv[1], "int") == 0) {
        printf("%lld\n", (long long)part_int(200000000));
    } else if (strcmp(argv[1], "sieve") == 0) {
        printf("%lld\n", (long long)part_sieve(50000000));
    } else if (strcmp(argv[1], "text") == 0) {
        printf("%lld\n", (long long)part_text(1000000));
    } else {
        fprintf(stderr, "usage: bench int|sieve|text\n");
        return 1;
    }
    return 0;
}
