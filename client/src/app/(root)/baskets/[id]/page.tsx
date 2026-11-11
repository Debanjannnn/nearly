import { Suspense } from "react";
import { BasketDetail } from "@/components/basket/BasketDetail";

export default async function Basket({ params }: PageProps<"/baskets/[id]">) {
  const { id } = await params;
  return (
    <Suspense>
      <BasketDetail id={id} />
    </Suspense>
  );
}
