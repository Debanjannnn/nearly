//! Internal ledger: token balances + venue positions, deposits in, settlement of outgoing calls.
use near_sdk::json_types::U128;
use near_sdk::serde_json;
use near_sdk::{env, near, require, AccountId, Gas, Promise, PromiseError, PromiseOrValue};

use crate::events::Event;
use crate::{Contract, ContractExt};

const GAS_SETTLE: Gas = Gas::from_tgas(10);

/// Where an amount is booked.
#[near(serializers = [json])]
#[derive(Clone, Debug, PartialEq)]
pub enum Slot {
    Balance(AccountId),
    Position(String),
}

impl Contract {
    fn slot(&self, slot: &Slot) -> u128 {
        match slot {
            Slot::Balance(t) => self.balances.get(t).copied().unwrap_or(0),
            Slot::Position(k) => self.positions.get(k).copied().unwrap_or(0),
        }
    }

    fn put(&mut self, slot: &Slot, v: u128) {
        match (slot, v) {
            (Slot::Balance(t), 0) => drop(self.balances.remove(t)),
            (Slot::Balance(t), v) => drop(self.balances.insert(t.clone(), v)),
            (Slot::Position(k), 0) => drop(self.positions.remove(k)),
            (Slot::Position(k), v) => drop(self.positions.insert(k.clone(), v)),
        }
    }

    pub(crate) fn debit(&mut self, slot: &Slot, amount: u128) {
        let cur = self.slot(slot);
        require!(cur >= amount, "insufficient balance");
        self.put(slot, cur - amount);
    }

    pub(crate) fn credit(&mut self, slot: &Slot, amount: u128) {
        let cur = self.slot(slot);
        self.put(
            slot,
            cur.checked_add(amount)
                .unwrap_or_else(|| env::panic_str("overflow")),
        );
    }

    /// Chains `on_settle`: refunds `debit` on failure, books `credit` on success.
    pub(crate) fn settle(
        &self,
        p: Promise,
        debit: Option<Slot>,
        amount: u128,
        credit: Option<(Slot, Option<U128>)>,
        transfer_call: bool,
    ) -> Promise {
        let (credit, credit_amount) = credit.map_or((None, None), |(s, a)| (Some(s), a));
        p.then(
            Self::ext(env::current_account_id())
                .with_static_gas(GAS_SETTLE)
                .on_settle(debit, U128(amount), credit, credit_amount, transfer_call),
        )
    }

    /// Debits the balance and sends an FT; refunded if the transfer fails.
    pub(crate) fn send(&mut self, token: AccountId, to: AccountId, amount: u128) -> Promise {
        let slot = Slot::Balance(token.clone());
        self.debit(&slot, amount);
        self.settle(
            crate::venues::ft_transfer(token, to, amount),
            Some(slot),
            amount,
            None,
            false,
        )
    }
}

#[near]
impl Contract {
    /// NEP-141 receiver: only whitelisted tokens; anyone may deposit.
    pub fn ft_on_transfer(
        &mut self,
        sender_id: AccountId,
        amount: U128,
        #[allow(unused_variables)] msg: String,
    ) -> PromiseOrValue<U128> {
        let token = env::predecessor_account_id();
        require!(self.caps.contains_key(&token), "token not whitelisted");
        self.credit(&Slot::Balance(token.clone()), amount.0);
        Event::Deposit {
            token,
            sender: sender_id,
            amount,
        }
        .emit();
        PromiseOrValue::Value(U128(0))
    }

    /// Returns the amount the venue actually took.
    #[private]
    pub fn on_settle(
        &mut self,
        debit: Option<Slot>,
        amount: U128,
        credit: Option<Slot>,
        credit_amount: Option<U128>,
        transfer_call: bool,
    ) -> U128 {
        // only a real failure refunds; a long success result still means funds left
        let used = match env::promise_result_checked(0, 64) {
            Err(PromiseError::Failed) => 0,
            Err(_) => amount.0,
            // ft_resolve_transfer returns used amount; unparsable = assume all spent
            Ok(b) if transfer_call => {
                serde_json::from_slice::<U128>(&b).map_or(amount.0, |u| u.0.min(amount.0))
            }
            Ok(_) => amount.0,
        };
        if let Some(d) = debit {
            if used < amount.0 {
                self.credit(&d, amount.0 - used);
            }
        }
        if let (Some(c), true) = (credit, used > 0) {
            self.credit(&c, credit_amount.map_or(used, |a| a.0));
        }
        U128(used)
    }
}
