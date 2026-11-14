#!/usr/bin/env python3
"""Writes <dir>/.env from .env.example once: random secrets + a fresh executor ed25519 key.
No third-party deps (openssl CLI for the key). Usage: init_env.py /opt/nearly"""
import os
import secrets
import subprocess
import sys
import tempfile

B58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"


def b58(b: bytes) -> str:
    n, s = int.from_bytes(b, "big"), ""
    while n:
        n, r = divmod(n, 58)
        s = B58[r] + s
    return "1" * (len(b) - len(b.lstrip(b"\0"))) + s


def near_keypair() -> tuple[str, str]:
    """NEAR format: secret = ed25519:b58(seed||pub), public = ed25519:b58(pub)."""
    with tempfile.TemporaryDirectory() as d:
        pem = os.path.join(d, "k.pem")
        subprocess.run(["openssl", "genpkey", "-algorithm", "ed25519", "-out", pem], check=True)
        # DER encodings end with the raw 32-byte seed / public key
        seed = subprocess.check_output(["openssl", "pkey", "-in", pem, "-outform", "DER"])[-32:]
        pub = subprocess.check_output(["openssl", "pkey", "-in", pem, "-pubout", "-outform", "DER"])[-32:]
    return f"ed25519:{b58(seed + pub)}", f"ed25519:{b58(pub)}"


def fill(example: str, values: dict) -> str:
    out = []
    for line in example.splitlines():
        k = line.split("=", 1)[0]
        out.append(f"{k}={values[k]}" if "=" in line and not line.startswith("#") and k in values else line)
    return "\n".join(out) + "\n"


def main(root: str) -> None:
    dst = os.path.join(root, ".env")
    if os.path.exists(dst):
        return
    priv, pub = near_keypair()
    pg = secrets.token_hex(16)
    values = {
        "EXECUTOR_ACCOUNT_ID": "nearly-executor",  # log label only; signer is each user's account
        "EXECUTOR_PRIVATE_KEY": priv,
        "EXECUTOR_PUBLIC_KEY": pub,
        "INTERNAL_SECRET": secrets.token_hex(32),
        "SESSION_SECRET": secrets.token_hex(32),
        "POSTGRES_PASSWORD": pg,
        "DATABASE_URL": f"postgres://nearly:{pg}@localhost:5432/nearly?sslmode=disable",
    }
    with open(os.path.join(root, ".env.example")) as f:
        body = fill(f.read(), values)
    fd = os.open(dst, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as f:
        f.write(body)


if __name__ == "__main__":
    main(sys.argv[1])
