// The same three workloads as bench.brick, written in Rust.
use std::env;
use std::process::exit;

fn part_int(n: i64) -> i64 {
    let mut x: i64 = 0;
    let mut i: i64 = 0;
    while i < n {
        x = (x * 31 + i) % 1000000007;
        i += 1;
    }
    x
}

fn part_sieve(n: usize) -> u64 {
    let mut flags = vec![1u8; n];
    let mut count: u64 = 0;
    let mut j = 2;
    while j < n {
        if flags[j] != 0 {
            count += 1;
            let mut k = j * 2;
            while k < n {
                flags[k] = 0;
                k += j;
            }
        }
        j += 1;
    }
    count
}

fn part_text(n: i64) -> usize {
    let mut s = String::new();
    let mut i: i64 = 0;
    while i < n {
        s.push_str(&i.to_string());
        s.push(',');
        i += 1;
    }
    s.len()
}

fn main() {
    let args: Vec<String> = env::args().collect();
    if args.len() < 2 {
        eprintln!("usage: bench int|sieve|text");
        exit(1);
    }
    match args[1].as_str() {
        "int" => println!("{}", part_int(200000000)),
        "sieve" => println!("{}", part_sieve(50000000)),
        "text" => println!("{}", part_text(1000000)),
        _ => {
            eprintln!("usage: bench int|sieve|text");
            exit(1);
        }
    }
}
