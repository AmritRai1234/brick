/*
 * The socket syscalls behind server.brick.
 *
 * Brick has no networking builtins, so this companion C file (linked
 * automatically because it is named after the program) provides the five
 * syscalls and the port lookup -- nothing else. The accept loop, the request
 * scan and the response all live in server.brick, so what the benchmark
 * measures above these calls is Brick's own code.
 */
#include <errno.h>
#include <netinet/in.h>
#include <signal.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

static char request[8192];

int64_t srv_port(void)
{
    const char *port = getenv("PORT");

    if (port == NULL || *port == '\0') {
        return 18080;
    }
    return (int64_t)strtoll(port, NULL, 10);
}

int64_t srv_listen(int64_t port)
{
    struct sockaddr_in addr;
    int                fd;
    int                one = 1;

    signal(SIGPIPE, SIG_IGN);
    fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd < 0) {
        return -1;
    }
    setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &one, sizeof one);
    memset(&addr, 0, sizeof addr);
    addr.sin_family      = AF_INET;
    addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    addr.sin_port        = htons((uint16_t)port);
    if (bind(fd, (struct sockaddr *)&addr, sizeof addr) < 0) {
        close(fd);
        return -1;
    }
    if (listen(fd, 512) < 0) {
        close(fd);
        return -1;
    }
    return fd;
}

int64_t srv_accept(int64_t fd)
{
    int cfd;

    do {
        cfd = accept((int)fd, NULL, NULL);
    } while (cfd < 0 && errno == EINTR);
    return cfd;
}

/* One blocking read into a static buffer, NUL-terminated: text in Brick. */
const char *srv_read(int64_t fd)
{
    ssize_t n;

    do {
        n = recv((int)fd, request, sizeof request - 1, 0);
    } while (n < 0 && errno == EINTR);
    if (n < 0) {
        n = 0;
    }
    request[n] = '\0';
    return request;
}

int64_t srv_send(int64_t fd, const char *data)
{
    size_t left = strlen(data);

    while (left > 0) {
        ssize_t n = send((int)fd, data, left, 0);
        if (n <= 0) {
            return -1;
        }
        data += n;
        left -= (size_t)n;
    }
    return 0;
}

int64_t srv_close(int64_t fd)
{
    return (int64_t)close((int)fd);
}
