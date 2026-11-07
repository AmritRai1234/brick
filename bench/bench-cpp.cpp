// The same three workloads as bench.brick, written in C++.
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <string>
#include <vector>

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
    std::vector<unsigned char> flags(static_cast<std::size_t>(n), 1);
    int64_t count = 0;

    for (int64_t j = 2; j < n; j++) {
        if (flags[static_cast<std::size_t>(j)]) {
            count++;
            for (int64_t k = j * 2; k < n; k += j) {
                flags[static_cast<std::size_t>(k)] = 0;
            }
        }
    }
    return count;
}

static int64_t part_text(int64_t n)
{
    std::string s;

    for (int64_t i = 0; i < n; i++) {
        s += std::to_string(i);
        s += ',';
    }
    return static_cast<int64_t>(s.size());
}

int main(int argc, char **argv)
{
    if (argc < 2) {
        std::fprintf(stderr, "usage: bench int|sieve|text\n");
        return 1;
    }
    if (std::strcmp(argv[1], "int") == 0) {
        std::printf("%lld\n", static_cast<long long>(part_int(200000000)));
    } else if (std::strcmp(argv[1], "sieve") == 0) {
        std::printf("%lld\n", static_cast<long long>(part_sieve(50000000)));
    } else if (std::strcmp(argv[1], "text") == 0) {
        std::printf("%lld\n", static_cast<long long>(part_text(1000000)));
    } else {
        std::fprintf(stderr, "usage: bench int|sieve|text\n");
        return 1;
    }
    return 0;
}
