import { IssueDetailPage } from "@/components/issue-detail-page";

export default async function IssuePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  return <IssueDetailPage issueID={id} />;
}
