import { MemoryDetailPage } from "@/components/memory-detail-page";

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const resolvedParams = await params;

  return <MemoryDetailPage memoryID={resolvedParams.id} />;
}
