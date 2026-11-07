/*
 * The same very small HTTP/1.1 server as server.brick, in plain C.
 *
 * Deliberately the same shape as the Brick program: one accept loop, one
 * blocking read per connection into a static buffer, one scan for the end of
 * the request line, one send, close. Comparing the two measures the cost of
 * Brick's generated code, not a difference in design.
 *
 * Build:
 *   cc -std=c11 -O2 -o bench/server/build/c-server bench/server/server-c.c
 */
#include <errno.h>
#include <netinet/in.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

static const char ok_response[] =
    "HTTP/1.1 200 OK\r\n"
    "Content-Type: text/plain\r\n"
    "Content-Length: 14\r\n"
    "Connection: close\r\n"
    "\r\n"
    "Hello, Brick!\n";

static const char bad_response[] =
    "HTTP/1.1 404 Not Found\r\n"
    "Content-Type: text/plain\r\n"
    "Content-Length: 4\r\n"
    "Connection: close\r\n"
    "\r\n"
    "nope";

static char request[8192];

static long parse_port(void)
{
    const char *port = getenv("PORT");

    if (port == NULL || *port == '\0') {
        return 18080;
    }
    return strtol(port, NULL, 10);
}

static int open_listener(long port)
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
    addr.sin_port        = htons((unsigned short)port);
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

/* Scan the request for the first CR: the line the method is on. */
static size_t request_line_length(void)
{
    size_t i = 0;

    while (request[i] != '\0') {
        if (request[i] == '\r') {
            return i;
        }
        i++;
    }
    return i;
}

static void send_all(int fd, const char *data)
{
    size_t left = strlen(data);

    while (left > 0) {
        ssize_t n = send(fd, data, left, 0);
        if (n <= 0) {
            return;
        }
        data += n;
        left -= (size_t)n;
    }
}

/* One request per connection: read, answer, close. */
static void handle(int cfd)
{
    size_t line;
    ssize_t n;

    do {
        n = recv(cfd, request, sizeof request - 1, 0);
    } while (n < 0 && errno == EINTR);
    if (n < 0) {
        n = 0;
    }
    request[n] = '\0';
    line = request_line_length();
    if (line >= 4 && memcmp(request, "GET ", 4) == 0) {
        send_all(cfd, ok_response);
    } else {
        send_all(cfd, bad_response);
    }
    close(cfd);
}

int main(void)
{
    int fd;
    int cfd;

    fd = open_listener(parse_port());
    if (fd < 0) {
        fprintf(stderr, "c-server: cannot listen\n");
        return 1;
    }
    fprintf(stderr, "c-server: listening\n");
    for (;;) {
        cfd = accept(fd, NULL, NULL);
        if (cfd < 0) {
            if (errno == EINTR) {
                continue;
            }
            return 1;
        }
        handle(cfd);
    }
}
