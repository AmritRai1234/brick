// The same very small HTTP/1.1 server as server.brick, in Rust.
//
// Same shape as the Brick, C and Go versions: one accept loop, one blocking
// read per connection, one scan for the CR, one write, close.
//
// Build:
//	rustc -O -o bench/server/build/rust-server bench/server/server-rust.rs
use std::env;
use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};

const OK_RESPONSE: &str = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 14\r\nConnection: close\r\n\r\nHello, Brick!\n";

const BAD_RESPONSE: &str = "HTTP/1.1 404 Not Found\r\nContent-Type: text/plain\r\nContent-Length: 4\r\nConnection: close\r\n\r\nnope";

fn port() -> u16 {
    match env::var("PORT") {
        Ok(p) => p.parse().unwrap_or(18080),
        Err(_) => 18080,
    }
}

// One request per connection: read, answer, close.
fn handle(mut stream: TcpStream) {
    let mut buf = [0u8; 8192];
    let n = stream.read(&mut buf).unwrap_or(0);
    let line = buf[..n]
        .iter()
        .position(|&b| b == b'\r' || b == 0)
        .unwrap_or(n);
    let response = if line >= 4 && &buf[..4] == b"GET " {
        OK_RESPONSE
    } else {
        BAD_RESPONSE
    };
    let _ = stream.write_all(response.as_bytes());
}

fn main() {
    let listener = match TcpListener::bind(("127.0.0.1", port())) {
        Ok(l) => l,
        Err(_) => {
            eprintln!("rust-server: cannot listen");
            std::process::exit(1);
        }
    };
    eprintln!("rust-server: listening");
    for stream in listener.incoming() {
        match stream {
            Ok(s) => handle(s),
            Err(_) => std::process::exit(1),
        }
    }
}
