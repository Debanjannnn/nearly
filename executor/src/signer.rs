//! ed25519 function-call key and NEAR transaction signing (borsh, hand-rolled: one action type).
use ed25519_dalek::{Signer as _, SigningKey};
use sha2::{Digest, Sha256};

pub struct Signer(SigningKey);

/// One FunctionCall action from `signer_id` to itself.
pub struct Call<'a> {
    pub account: &'a str,
    pub nonce: u64,
    pub block_hash: [u8; 32],
    pub method: &'a str,
    pub args: &'a [u8],
    pub gas: u64,
}

pub struct Signed {
    pub hash: String,
    pub bytes: Vec<u8>,
}

impl Signer {
    /// `ed25519:<base58>` of the 64-byte keypair or the 32-byte seed.
    pub fn parse(s: &str) -> Result<Self, String> {
        let b = bs58::decode(
            s.strip_prefix("ed25519:")
                .ok_or("key must start with ed25519:")?,
        )
        .into_vec()
        .map_err(|_| "key is not base58")?;
        let seed: [u8; 32] = b
            .get(..32)
            .filter(|_| b.len() == 32 || b.len() == 64)
            .ok_or("bad key length")?
            .try_into()
            .unwrap();
        let k = SigningKey::from_bytes(&seed);
        if b.len() == 64 && b[32..] != k.verifying_key().to_bytes() {
            return Err("key halves do not match".into());
        }
        Ok(Self(k))
    }

    pub fn public_key(&self) -> String {
        format!(
            "ed25519:{}",
            bs58::encode(self.0.verifying_key().to_bytes()).into_string()
        )
    }

    pub fn sign(&self, c: &Call) -> Signed {
        let mut tx = Vec::with_capacity(256);
        string(&mut tx, c.account);
        tx.push(0); // KeyType::ED25519
        tx.extend(self.0.verifying_key().to_bytes());
        tx.extend(c.nonce.to_le_bytes());
        string(&mut tx, c.account);
        tx.extend(c.block_hash);
        tx.extend(1u32.to_le_bytes()); // one action
        tx.push(2); // Action::FunctionCall
        string(&mut tx, c.method);
        tx.extend((c.args.len() as u32).to_le_bytes());
        tx.extend(c.args);
        tx.extend(c.gas.to_le_bytes());
        tx.extend(0u128.to_le_bytes()); // deposit: fc keys cannot attach any
        let hash: [u8; 32] = Sha256::digest(&tx).into();
        tx.push(0); // Signature::ED25519
        tx.extend(self.0.sign(&hash).to_bytes());
        Signed {
            hash: bs58::encode(hash).into_string(),
            bytes: tx,
        }
    }
}

fn string(out: &mut Vec<u8>, s: &str) {
    out.extend((s.len() as u32).to_le_bytes());
    out.extend(s.as_bytes());
}

#[cfg(test)]
mod tests {
    use super::*;
    use ed25519_dalek::{Signature, Verifier};

    #[test]
    fn parse_and_sign() {
        let seed = [7u8; 32];
        let pk = SigningKey::from_bytes(&seed).verifying_key().to_bytes();
        let full = format!(
            "ed25519:{}",
            bs58::encode([seed, pk].concat()).into_string()
        );
        let s = Signer::parse(&full).unwrap();
        assert_eq!(
            s.public_key(),
            Signer::parse(&format!("ed25519:{}", bs58::encode(seed).into_string()))
                .unwrap()
                .public_key()
        );
        assert!(Signer::parse(&full.replace("ed25519:", "")).is_err());
        assert!(Signer::parse(&format!(
            "ed25519:{}",
            bs58::encode([seed, seed].concat()).into_string()
        ))
        .is_err());

        let c = Call {
            account: "a.testnet",
            nonce: 5,
            block_hash: [1; 32],
            method: "execute_step",
            args: b"{}",
            gas: 300_000_000_000_000,
        };
        let t = s.sign(&c);
        let (body, sig) = t.bytes.split_at(t.bytes.len() - 65);
        let h: [u8; 32] = Sha256::digest(body).into();
        assert_eq!(t.hash, bs58::encode(h).into_string());
        assert_eq!(sig[0], 0);
        s.0.verifying_key()
            .verify(&h, &Signature::from_bytes(sig[1..].try_into().unwrap()))
            .unwrap();
        // 4+9 signer, 33 pk, 8 nonce, 4+9 receiver, 32 hash, 4 actions, 1 tag, 4+12 method, 4+2 args, 8 gas, 16 deposit
        assert_eq!(body.len(), 13 + 33 + 8 + 13 + 32 + 4 + 1 + 16 + 6 + 8 + 16);
    }
}
