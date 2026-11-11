"use client";

import { Button, Card, ErrorBox, Row } from "@/components/ui";
import { NETWORK } from "@/lib/config";
import { useSession } from "@/lib/session";

export default function Home() {
  const { ready, accountId, account, authenticated, pending, error, login } = useSession();
  return (
    <main className="mx-auto max-w-xl space-y-4 p-4 md:p-6">
      <h1 className="text-3xl font-semibold tracking-tight">Yield baskets on NEAR, from your own account.</h1>
      <Card title="Your account">
        <dl className="space-y-2 text-sm">
          <Row label="Network">NEAR {NETWORK}</Row>
          <Row label="Wallet">{accountId ?? "not connected"}</Row>
          <Row label="Account">{account ?? "not created"}</Row>
        </dl>
        {!(authenticated && account) && (
          <Button className="mt-4 w-full" disabled={!ready} pending={pending} onClick={() => void login()}>
            {!accountId ? "Connect wallet" : !authenticated ? "Sign in" : "Create account"}
          </Button>
        )}
        {error && (
          <div className="mt-3">
            <ErrorBox message={error} />
          </div>
        )}
      </Card>
    </main>
  );
}
