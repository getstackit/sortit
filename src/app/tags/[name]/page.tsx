import { TagDetailPage } from "@/components/tag-detail-page";

export default async function TagPage({
  params,
}: {
  params: Promise<{ name: string }>;
}) {
  const { name } = await params;

  return <TagDetailPage tagName={name} />;
}
