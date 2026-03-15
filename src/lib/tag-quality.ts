import type { IssueRecord } from "@/lib/issues";
import type { TagRecord } from "@/lib/tags";

export type SpecificTagSuggestion = {
  name: string;
  description: string;
  relevance: number;
  count: number;
};

export type SpecificityLadderEntry = {
  name: string;
  description: string;
  similarity: number;
  specificity: number;
};

export type SpecificityLadder = {
  moreSpecific: SpecificityLadderEntry[];
  moreGeneric: SpecificityLadderEntry[];
};

export type MergeCandidate = {
  name: string;
  description: string;
  similarity: number;
  reason: string;
};

export type ConsolidationCandidate = {
  canonicalName: string;
  canonicalDescription: string;
  aliasName: string;
  aliasDescription: string;
  similarity: number;
  sharedIssueCount: number;
  containment: number;
  jaccard: number;
  score: number;
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

const CONSOLIDATION_MIN_RELEVANCE = 0.35;

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

function tagSpecificity(tag: TagRecord): number {
  return tag.specificity ?? 0.5;
}

export function buildSpecificityLadder(
  tagName: string,
  tags: TagRecord[],
  limit = 4
): SpecificityLadder {
  const normalizedName = normalizeTagName(tagName);
  const selectedTag = tags.find(
    (tag) => normalizeTagName(tag.name) === normalizedName
  );

  if (
    !selectedTag ||
    !Array.isArray(selectedTag.embedding) ||
    selectedTag.embedding.length <= 1
  ) {
    return { moreSpecific: [], moreGeneric: [] };
  }

  const selectedSpecificity = tagSpecificity(selectedTag);

  const similar = tags
    .filter((tag) => {
      if (normalizeTagName(tag.name) === normalizedName) return false;
      if (!Array.isArray(tag.embedding) || tag.embedding.length <= 1) return false;
      if (tag.embedding.length !== selectedTag.embedding.length) return false;
      return true;
    })
    .map((tag) => ({
      name: tag.name,
      description: tag.description ?? "",
      similarity: cosineSimilarity(selectedTag.embedding, tag.embedding),
      specificity: tagSpecificity(tag),
    }))
    .filter((entry) => entry.similarity >= 0.5);

  const moreSpecific = similar
    .filter((entry) => entry.specificity > selectedSpecificity)
    .sort((a, b) => b.specificity - a.specificity)
    .slice(0, limit);

  const moreGeneric = similar
    .filter((entry) => entry.specificity < selectedSpecificity)
    .sort((a, b) => a.specificity - b.specificity)
    .slice(0, limit);

  return { moreSpecific, moreGeneric };
}

export function buildSpecificTagSuggestions(
  tagName: string,
  tags: TagRecord[],
  limit = 4
): SpecificityLadderEntry[] {
  const normalizedName = normalizeTagName(tagName);
  const selectedTag = tags.find(
    (tag) => normalizeTagName(tag.name) === normalizedName
  );

  if (!selectedTag || tagSpecificity(selectedTag) >= 0.4) {
    return [];
  }

  const ladder = buildSpecificityLadder(tagName, tags, limit);
  return ladder.moreSpecific;
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

function relevantIssueIds(issues: IssueRecord[], tagName: string) {
  const ids = new Set<string>();
  for (const issue of issues) {
    if (issueTagRelevance(issue, tagName) >= CONSOLIDATION_MIN_RELEVANCE) {
      ids.add(issue.id);
    }
  }
  return ids;
}

function setIntersectionSize(left: Set<string>, right: Set<string>) {
  let count = 0;
  const smaller = left.size <= right.size ? left : right;
  const larger = smaller === left ? right : left;
  for (const value of smaller) {
    if (larger.has(value)) {
      count += 1;
    }
  }
  return count;
}

function compareCanonicalPreference(
  left: { tag: TagRecord; issueIds: Set<string> },
  right: { tag: TagRecord; issueIds: Set<string> }
) {
  if (left.issueIds.size !== right.issueIds.size) {
    return right.issueIds.size - left.issueIds.size;
  }

  const leftCreated = new Date(left.tag.createdAt).getTime();
  const rightCreated = new Date(right.tag.createdAt).getTime();
  if (leftCreated !== rightCreated) {
    return leftCreated - rightCreated;
  }

  if (left.tag.name.length !== right.tag.name.length) {
    return left.tag.name.length - right.tag.name.length;
  }

  return left.tag.name.localeCompare(right.tag.name);
}

function consolidationReason(
  lexicalVariant: boolean,
  similarity: number,
  sharedIssueCount: number,
  containment: number
) {
  if (lexicalVariant && sharedIssueCount > 0) {
    return "Name variant with overlapping issue coverage";
  }
  if (lexicalVariant) {
    return "Name variant";
  }
  if (similarity >= 0.97 && containment >= 0.8) {
    return "Near-synonym with almost identical issue coverage";
  }
  if (similarity >= 0.94 && sharedIssueCount >= 3) {
    return "High semantic overlap with repeated co-occurrence";
  }
  return "Semantic overlap with shared issue coverage";
}

export function buildConsolidationCandidates(
  tags: TagRecord[],
  issues: IssueRecord[],
  limit = 8
): ConsolidationCandidate[] {
  if (tags.length < 2) {
    return [];
  }

  const normalizedTags = tags.map((tag) => ({
    tag,
    normalizedName: normalizeTagName(tag.name),
    key: canonicalTagKey(tag.name),
    issueIds: relevantIssueIds(issues, tag.name),
  }));

  const candidates: ConsolidationCandidate[] = [];

  for (let leftIndex = 0; leftIndex < normalizedTags.length; leftIndex += 1) {
    const left = normalizedTags[leftIndex];

    for (let rightIndex = leftIndex + 1; rightIndex < normalizedTags.length; rightIndex += 1) {
      const right = normalizedTags[rightIndex];
      const lexicalVariant = left.key === right.key;
      const similarity =
        left.tag.embedding.length > 1 &&
        left.tag.embedding.length === right.tag.embedding.length
          ? cosineSimilarity(left.tag.embedding, right.tag.embedding)
          : 0;
      const sharedIssueCount = setIntersectionSize(left.issueIds, right.issueIds);
      const unionCount = left.issueIds.size + right.issueIds.size - sharedIssueCount;
      const jaccard = unionCount > 0 ? sharedIssueCount / unionCount : 0;
      const containmentBase = Math.min(left.issueIds.size, right.issueIds.size);
      const containment = containmentBase > 0 ? sharedIssueCount / containmentBase : 0;

      const qualifies =
        lexicalVariant ||
        (similarity >= 0.97 && containment >= 0.6 && sharedIssueCount >= 2) ||
        (similarity >= 0.94 && jaccard >= 0.5 && sharedIssueCount >= 3);

      if (!qualifies) {
        continue;
      }

      const preferred = [left, right].sort(compareCanonicalPreference);
      const canonical = preferred[0];
      const alias = preferred[1];
      const score =
        similarity * 0.55 +
        containment * 0.25 +
        jaccard * 0.15 +
        (lexicalVariant ? 0.25 : 0);

      candidates.push({
        canonicalName: canonical.tag.name,
        canonicalDescription: canonical.tag.description ?? "",
        aliasName: alias.tag.name,
        aliasDescription: alias.tag.description ?? "",
        similarity,
        sharedIssueCount,
        containment,
        jaccard,
        score,
        reason: consolidationReason(lexicalVariant, similarity, sharedIssueCount, containment),
      });
    }
  }

  return candidates
    .sort((left, right) => {
      if (right.score !== left.score) {
        return right.score - left.score;
      }
      if (right.sharedIssueCount !== left.sharedIssueCount) {
        return right.sharedIssueCount - left.sharedIssueCount;
      }
      return left.aliasName.localeCompare(right.aliasName);
    })
    .slice(0, limit);
}
