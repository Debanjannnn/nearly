import { Suspense } from "react";
import { BasketList } from "@/components/basket/BasketList";

export default function Baskets() {
  return (
    <Suspense>
      <BasketList />
    </Suspense>
  );
}
