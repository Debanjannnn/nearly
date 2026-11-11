import { Card } from "@/components/ui";

export default async function Basket({ params }: PageProps<"/baskets/[id]">) {
  const { id } = await params;
  return (
    <main className="p-4 md:p-6">
      <Card title={`Basket ${id}`}>
        <p className="text-sm text-muted-foreground">Basket details coming soon.</p>
      </Card>
    </main>
  );
}
