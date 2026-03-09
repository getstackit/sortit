import type { IssueRecord } from "@/lib/issues";
import type { TagRecord } from "@/lib/tags";

export type SpecificTagSuggestion = {
  name: string;
  description: string;
  relevance: number;
  count: number;
};

export type MergeCandidate = {
  name: string;
  description: string;
  similarity: number;
  reason: string;
};

const GENERIC_BUCKET_TAGS = new Set([
  "api",
  "backend",
  "client",
  "front end",
  "frontend",
  "server",
  "ui",
  "ux",
]);

export function normalizeTagName(value: string) {
  return value.trim().toLowerCase();
}

function canonicalTagKey(value: string) {
  return normalizeTagName(value).replace(/[\s/_-]+/g, "");
}

export function isGenericBucketTag(name: string) {
  return GENERIC_BUCKET_TAGS.has(normalizeTagName(name));
}

export function cosineSimilarity(left: number[], right: number[]) {
  if (left.length === 0 || left.length !== right.length) {
    return 0;
  }

  let dotProduct = 0;
  let leftMagnitude = 0;
  let rightMagnitude = 0;
  for (let index = 0; index < left.length; index += 1) {
    dotProduct += left[index] * right[index];
    leftMagnitude += left[index] * left[index];
    rightMagnitude += right[index] * right[index];
  }

  if (leftMagnitude === 0 || rightMagnitude === 0) {
    return 0;
  }

  return dotProduct / (Math.sqrt(leftMagnitude) * Math.sqrt(rightMagnitude));
}

export function issueTagRelevance(issue: IssueRecord, tagName: string) {
  const normalizedTag = normalizeTagName(tagName);
  const score = issue.tagScores?.find(
    (entry) => normalizeTagName(entry.tag) === normalizedTag
  );
  if (score) {
    return score.relevance;
  }

  return issue.tags.some((tag) => normalizeTagName(tag) === normalizedTag) ? 1 : 0;
}

export function buildSpecificTagSuggestions(
  tagName: string,
  tags: TagRecord[],
  issues: IssueRecord[],
  limit = 6
): SpecificTagSuggestion[] {
  if (!isGenericBucketTag(tagName)) {
    return [];
  }

  const tagsByName = new Map(
    tags.map((tag) => [normalizeTagName(tag.name), tag] as const)
  );
  const selectedTag = normalizeTagName(tagName);
  const aggregate = new Map<string, { total: number; count: number }>();

  for (const issue of issues) {
    if (issueTagRelevance(issue, selectedTag) <= 0) {
      continue;
    }

    const sourceTags =
      issue.tagScores && issue.tagScores.length > 0
        ? issue.tagScores.map((entry) => ({
            tag: entry.tag,
            relevance: entry.relevance,
          }))
        : issue.tags.map((entry) => ({ tag: entry, relevance: 1 }));

    for (const entry of sourceTags) {
      const normalizedEntry = normalizeTagName(entry.tag);
      if (normalizedEntry === selectedTag || isGenericBucketTag(normalizedEntry)) {
        continue;
      }

      const current = aggregate.get(normalizedEntry) ?? { total: 0, count: 0 };
      current.total += entry.relevance;
      current.count += 1;
      aggregate.set(normalizedEntry, current);
    }
  }

  return Array.from(aggregate.entries())
    .map(([normalizedEntry, stats]) => {
      const tag = tagsByName.get(normalizedEntry);
      return {
        name: tag?.name ?? normalizedEntry,
        description: tag?.description ?? "",
        relevance: stats.total / stats.count,
        count: stats.count,
      };
    })
    .sort((left, right) => {
      if (right.relevance !== left.relevance) {
        return right.relevance - left.relevance;
      }
      if (right.count !== left.count) {
        return right.count - left.count;
      }
      return left.name.localeCompare(right.name);
    })
    .slice(0, limit);
}

export function buildMergeCandidates(
  selectedTag: TagRecord | null,
  tags: TagRecord[],
  limit = 6
): MergeCandidate[] {
  if (!selectedTag) {
    return [];
  }

  const selectedName = normalizeTagName(selectedTag.name);
  const selectedKey = canonicalTagKey(selectedTag.name);

  return tags
    .filter((candidate) => normalizeTagName(candidate.name) !== selectedName)
    .map((candidate) => {
      const lexicalVariant = canonicalTagKey(candidate.name) === selectedKey;
      const similarity =
        selectedTag.embedding.length > 1 &&
        selectedTag.embedding.length === candidate.embedding.length
          ? cosineSimilarity(selectedTag.embedding, candidate.embedding)
          : 0;

      let reason = "";
      if (lexicalVariant) {
        reason = "Name variant";
      } else if (similarity >= 0.94) {
        reason = "Very high semantic overlap";
      } else if (similarity >= 0.88) {
        reason = "High semantic overlap";
      }

      return {
        name: candidate.name,
        description: candidate.description ?? "",
        similarity,
        reason,
        lexicalVariant,
      };
    })
    .filter((candidate) => candidate.reason !== "")
    .sort((left, right) => {
      if (Number(right.lexicalVariant) !== Number(left.lexicalVariant)) {
        return Number(right.lexicalVariant) - Number(left.lexicalVariant);
      }
      if (right.similarity !== left.similarity) {
        return right.similarity - left.similarity;
      }
      return left.name.localeCompare(right.name);
    })
    .slice(0, limit)
    .map((candidate) => ({
      name: candidate.name,
      description: candidate.description,
      similarity: candidate.similarity,
      reason: candidate.reason,
    }));
}
