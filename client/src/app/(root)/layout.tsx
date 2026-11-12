import { Suspense } from "react";
import { Navbar } from "@/components/Navbar";
import { GridBackdrop } from "@/components/ui/grid-pattern";

export default function RootGroupLayout({ children }: LayoutProps<"/">) {
  return (
    <div className="flex min-h-full w-full flex-col items-center">
      <GridBackdrop />
      <Suspense>
        <Navbar />
      </Suspense>
      <div className="w-full max-w-[1400px]">{children}</div>
    </div>
  );
}
