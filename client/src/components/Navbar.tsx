"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { Check, Copy, Menu, Wallet as WalletIcon } from "lucide-react";
import { shortId } from "@/lib/api";
import { accountUrl, NETWORK } from "@/lib/config";
import { useCopy } from "@/lib/hooks";
import { useSession } from "@/lib/session";
import { cn } from "@/lib/utils";
import { BasketSearch } from "./BasketSearch";
import { Row, Spinner, useDismissable } from "./ui";

const NAV = [
  { label: "Baskets", href: "/baskets" },
  { label: "Portfolio", href: "/portfolio" },
] as const;

export function Navbar() {
  const pathname = usePathname();
  const { ready, accountId, authenticated, account, login, pending } = useSession();
  const { open: menuOpen, setOpen: setMenuOpen, ref: menuRef } = useDismissable();
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    const on = () => setScrolled(window.scrollY > 4);
    on();
    window.addEventListener("scroll", on, { passive: true });
    return () => window.removeEventListener("scroll", on);
  }, []);
  const links = (cls: string) =>
    NAV.map((item) => (
      <Link
        key={item.href}
        href={item.href}
        onClick={() => setMenuOpen(false)}
        className={cn(
          cls,
          "font-medium transition-colors duration-150",
          pathname.startsWith(item.href) ? "text-foreground" : "text-muted-foreground hover:text-foreground",
        )}
      >
        {item.label}
      </Link>
    ));

  return (
    // transparent at the top of every page; solid + hairline once scrolled
    <header
      className={cn(
        "sticky top-0 z-40 w-full border-b transition-colors duration-200",
        scrolled ? "border-border bg-background/90 backdrop-blur-md" : "border-transparent bg-transparent",
      )}
    >
      <div className="flex h-16 w-full items-center justify-between gap-4 px-4 md:px-6">
        <Link href="/" className="relative text-2xl font-semibold tracking-tight">
          nearly
          <span className="absolute top-0 left-full ml-0.5 text-[10px] leading-none font-medium tracking-normal text-primary">{NETWORK}</span>
        </Link>

        <div className="flex items-center justify-end gap-2 sm:gap-6">
          <BasketSearch />
          <nav aria-label="Main navigation" className="hidden items-center gap-6 md:flex">
            {links("inline-flex h-10 items-center px-2 text-[15px]")}
          </nav>
          <div className="relative md:hidden" ref={menuRef}>
            <button
              onClick={() => setMenuOpen((o) => !o)}
              aria-expanded={menuOpen}
              aria-label="Menu"
              className="grid size-9 place-items-center rounded-full text-muted-foreground transition-colors hover:text-foreground"
            >
              <Menu className="size-5" />
            </button>
            {menuOpen && (
              <nav className="absolute right-0 top-full z-50 mt-2 flex w-44 flex-col rounded-xl border border-border bg-popover p-1 shadow-xl">
                {links("rounded-lg px-3 py-2 text-sm hover:bg-secondary")}
              </nav>
            )}
          </div>
          {!ready ? (
            <Spinner />
          ) : accountId && authenticated && account ? (
            <WalletMenu />
          ) : (
            <button
              onClick={() => void login()}
              disabled={pending}
              className="inline-flex h-9 items-center gap-2 rounded-full bg-primary px-3.5 text-sm font-semibold whitespace-nowrap text-primary-foreground transition-[background-color,scale] duration-150 hover:bg-primary/90 active:scale-[0.96] disabled:opacity-70"
            >
              {pending && <span className="size-3 animate-spin rounded-full border-2 border-current/30 border-t-current" />}
              {!accountId ? "Connect wallet" : !authenticated ? "Sign in" : "Create account"}
            </button>
          )}
        </div>
      </div>
    </header>
  );
}

function WalletMenu() {
  const { accountId, account, logout, error, keyStatus } = useSession();
  const { open, setOpen, ref } = useDismissable();

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-label="Wallet"
        className="tnum inline-flex h-9 shrink-0 items-center gap-2 whitespace-nowrap rounded-full bg-secondary px-3.5 text-sm font-medium transition-colors hover:bg-accent"
      >
        <WalletIcon className="size-4 text-muted-foreground" />
        {shortId(accountId ?? "")}
      </button>
      {open && (
        <div className="absolute right-0 top-full z-50 mt-2 w-80 max-w-[calc(100vw-2rem)] space-y-3 rounded-xl border border-border bg-popover p-4 text-sm shadow-xl">
          <dl className="space-y-1">
            {accountId && <AccountRow label="Wallet" id={accountId} />}
            {account && <AccountRow label="Account" id={account} />}
            <Row label="Network">NEAR {NETWORK}</Row>
            {keyStatus && <Row label="Automation key">{keyStatus}</Row>}
          </dl>
          {error && <p className="text-xs break-words text-destructive">{error}</p>}
          <button
            onClick={() => void logout()}
            className="rounded-lg border border-border bg-secondary/50 px-3 py-1.5 text-xs transition-colors hover:bg-secondary"
          >
            Log out
          </button>
        </div>
      )}
    </div>
  );
}

// short account id → explorer, with copy
function AccountRow({ label, id }: { label: string; id: string }) {
  const { copied, copy } = useCopy();
  return (
    <div className="flex items-center justify-between gap-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="flex items-center gap-0.5">
        <a href={accountUrl(id)} target="_blank" rel="noreferrer" title={id} className="tnum underline-offset-4 hover:underline">
          {shortId(id)}
        </a>
        <button
          onClick={() => copy(id)}
          aria-label={`Copy ${label.toLowerCase()} id`}
          className="rounded-md p-1.5 text-muted-foreground transition-colors hover:text-foreground"
        >
          {copied === id ? <Check className="size-3.5 text-positive" /> : <Copy className="size-3.5" />}
        </button>
      </dd>
    </div>
  );
}
