import type { ReactNode } from "react";
import { Skeleton } from "@/components/ui/skeleton";

type DetailPageLayoutProps = {
  loading?: boolean;
  error?: ReactNode;
  children: ReactNode;
};

export function DetailPageLayout({ loading, error, children }: DetailPageLayoutProps) {
  return (
    <div className="app-scrollarea min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 lg:px-6 xl:px-8">
        {loading && <DetailPageSkeleton />}
        {!loading && error && <div className="app-status-warning">{error}</div>}
        {!loading && !error && children}
      </div>
    </div>
  );
}

function DetailPageSkeleton() {
  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_21rem]">
      <div className="space-y-6">
        <Skeleton className="h-40 rounded-[1.75rem]" />
        <Skeleton className="h-96 rounded-[1.75rem]" />
      </div>
      <div className="space-y-4">
        <Skeleton className="h-56 rounded-[1.5rem]" />
        <Skeleton className="h-64 rounded-[1.5rem]" />
      </div>
    </div>
  );
}

type DetailPageGridProps = {
  children: ReactNode;
  sidebar: ReactNode;
};

export function DetailPageGrid({ children, sidebar }: DetailPageGridProps) {
  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_21rem] 2xl:grid-cols-[minmax(0,1.1fr)_24rem]">
      <div className="space-y-6">{children}</div>
      <aside className="space-y-4 xl:sticky xl:top-6 xl:self-start">{sidebar}</aside>
    </div>
  );
}
