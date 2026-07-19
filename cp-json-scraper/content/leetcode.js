// content/leetcode.js
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.action === "START_SCRAPING") {
    scrapeLeetCode().then((payload) => {
      if (payload) {
        sendResponse({ success: true, data: payload });
      } else {
        sendResponse({
          success: false,
          error: "Failed to scrape LeetCode problem page.",
        });
      }
    });
  }
  return true; // Keep channel open
});

async function scrapeLeetCode() {
  try {
    const title =
      document.querySelector("span.text-title-large")?.innerText ||
      "LeetCode Problem";

    // Extract test cases directly from the description container's text.
    // Older LeetCode problem pages wrap each "Input:/Output:" example in a
    // single <pre> block, but newer redesigned pages (like this one) print
    // "Example N:", "Input:", "Output:", "Explanation:" as separate plain
    // lines with no <pre> at all -- querying for <pre> then finds nothing
    // and the scraper silently returns zero tests. innerText flattens
    // whatever markup is actually used either way, so splitting on
    // "Example N:" markers over the raw text works for both formats.
    const descContainer = document.querySelector(
      'div[data-track-load="description_content"]',
    );
    const fullText = descContainer ? descContainer.innerText : "";

    let tests = [];
    const exampleMarkers = [...fullText.matchAll(/Example\s*\d+:?/gi)];

    for (let i = 0; i < exampleMarkers.length; i++) {
      const start = exampleMarkers[i].index;
      const end =
        i + 1 < exampleMarkers.length
          ? exampleMarkers[i + 1].index
          : fullText.length;
      const chunk = fullText.slice(start, end);

      const inputMatch = chunk.match(/Input:\s*([\s\S]*?)(?=Output:)/i);
      const outputMatch = chunk.match(
        /Output:\s*([\s\S]*?)(?=Explanation:|Constraints:|$)/i,
      );

      if (inputMatch && outputMatch) {
        tests.push({
          input: inputMatch[1].trim() + "\n",
          expected: outputMatch[1].trim() + "\n",
        });
      }
    }

    // Fallback for any layout this doesn't anticipate: the old <pre>-based
    // extraction, in case a page mixes formats or the container selector
    // above changes again in the future.
    if (tests.length === 0) {
      const preBlocks = document.querySelectorAll(
        'div[data-track-load="description_content"] pre',
      );
      preBlocks.forEach((block) => {
        const text = block.innerText;
        if (text.includes("Input:") && text.includes("Output:")) {
          const inputMatch = text.match(/Input:\s*([\s\S]*?)(?=Output:)/);
          const outputMatch = text.match(
            /Output:\s*([\s\S]*?)(?=Explanation:|Example|\n\n|$)/,
          );
          if (inputMatch && outputMatch) {
            tests.push({
              input: inputMatch[1].trim() + "\n",
              expected: outputMatch[1].trim() + "\n",
            });
          }
        }
      });
    }

    if (tests.length === 0) {
      return null;
    }

    // Smart Data Structure verification via Topic Pill tags
    let dataStructure = "standard";
    const topicLinks = document.querySelectorAll("a");

    for (let link of topicLinks) {
      const href = link.href.toLowerCase();
      const tagText = link.innerText.toLowerCase();

      if (href.includes("/tag/binary-tree") || tagText === "binary tree") {
        dataStructure = "binary_tree";
        break;
      } else if (
        href.includes("/tag/linked-list") ||
        tagText === "linked list"
      ) {
        dataStructure = "linked_list";
        break;
      }
    }

    // Get snippets from GraphQL API using URL titleSlug
    // URL pattern is https://leetcode.com/problems/two-sum/
    const slug = window.location.pathname.split("/")[2];
    let signature = "";
    if (slug) {
      const snippets = await getSnippets(slug);
      // Try to find Go/Golang snippet first
      const goSnippet = snippets.find((s) => s.langSlug === "golang");
      if (goSnippet && goSnippet.code) {
        signature = extractSignature(goSnippet.code);
      } else {
        // Fallback to any snippet if Go is not available
        const firstSnippet = snippets[0];
        if (firstSnippet && firstSnippet.code) {
          signature = extractSignature(firstSnippet.code);
        }
      }
    }

    // If GraphQL failed or returned empty signature, fall back to editor scraping
    if (!signature) {
      const editorCode = getLeetCodeCode();
      signature = extractSignature(editorCode);
    }

    // Detect "in-place" problems (e.g. #31 Next Permutation, #88 Merge
    // Sorted Array, #26/#27/#80 Remove Duplicates/Element, #189 Rotate
    // Array, #283 Move Zeroes). These mutate an argument instead of (or as
    // well as) returning a value, which the CLI's default harness can't
    // check correctly — it needs to know to read the mutated argument back
    // afterward rather than trust the function's own return value alone.
    const { inPlace, targetParam } = detectInPlace(signature);

    return {
      platform: "leetcode",
      title: title,
      type: "function",
      data_structure: dataStructure,
      time_limit_ms: 2000,
      memory_limit_mb: 256,
      tests: tests,
      function_signature: signature,
      in_place: inPlace,
      target_param: targetParam,
    };
  } catch (err) {
    console.error("LeetCode scraping failed:", err);
    return null;
  }
}

// detectInPlace looks for two independent signals that a problem expects an
// in-place modification, and combines them with a best-guess at which
// parameter gets mutated:
//
//  1. Explicit text: the description literally says "in-place" / "in
//     place" (case-insensitive) -- LeetCode almost always links this phrase
//     straight to https://en.wikipedia.org/wiki/In-place_algorithm, so that
//     link is checked too as stronger confirmation than the bare phrase.
//  2. A best-guess target parameter: the first slice/array-typed parameter
//     in the signature (e.g. "nums" in "nums []int, target int") -- this
//     matches every well-known LeetCode in-place problem (26, 27, 31, 80,
//     88, 189, 283), since the mutated argument is always the first array
//     parameter.
//
// This is a heuristic default, not a guarantee -- the popup UI lets the
// user flip in_place and set target_param manually for anything this
// misses or gets wrong.
function detectInPlace(signature) {
  const bodyText = (
    document.querySelector('div[data-track-load="description_content"]')
      ?.innerText || ""
  ).toLowerCase();

  const mentionsInPlace =
    bodyText.includes("in-place") || bodyText.includes("in place");

  const hasWikipediaLink = Array.from(document.querySelectorAll("a")).some(
    (a) => (a.href || "").includes("wikipedia.org/wiki/In-place_algorithm"),
  );

  const inPlace = mentionsInPlace || hasWikipediaLink;

  let targetParam = "";
  if (inPlace) {
    targetParam = guessFirstSliceParam(signature);
  }

  return { inPlace, targetParam };
}

// guessFirstSliceParam extracts the first parameter whose type looks like a
// Go slice (e.g. "[]int") from a signature string like
// "func merge(nums1 []int, m int, nums2 []int, n int)". Returns "" if none
// is found or the signature can't be parsed, leaving target_param empty so
// the CLI falls back to its own auto-detection (or the user sets it
// manually in the popup).
function guessFirstSliceParam(signature) {
  const parenStart = signature.indexOf("(");
  const parenEnd = signature.indexOf(")");
  if (parenStart === -1 || parenEnd === -1 || parenEnd <= parenStart) {
    return "";
  }
  const paramsStr = signature.slice(parenStart + 1, parenEnd);
  const params = paramsStr.split(",").map((p) => p.trim());

  for (const param of params) {
    // Each param looks like "name []int" or "name int" etc.
    const parts = param.split(/\s+/);
    if (parts.length >= 2 && parts[1].startsWith("[]")) {
      return parts[0];
    }
  }
  return "";
}

async function getSnippets(titleSlug) {
  try {
    const res = await fetch("https://leetcode.com/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({
        query: `
          query questionEditorData($titleSlug: String!) {
            question(titleSlug: $titleSlug) {
              codeSnippets {
                lang
                langSlug
                code
              }
            }
          }`,
        variables: { titleSlug },
      }),
    });
    const data = await res.json();
    return data?.data?.question?.codeSnippets || [];
  } catch (err) {
    console.error("Failed to fetch code snippets from GraphQL:", err);
    return [];
  }
}

function getLeetCodeCode() {
  // Try CodeMirror 6 (new LeetCode UI)
  const cmContent = document.querySelector(".cm-content");
  if (cmContent) {
    return cmContent.innerText;
  }
  // Try Monaco Editor
  const monacoLines = document.querySelectorAll(".view-line");
  if (monacoLines && monacoLines.length > 0) {
    return Array.from(monacoLines)
      .map((l) => l.innerText)
      .join("\n");
  }
  // Alternate selectors
  const codeMirrorLines = document.querySelectorAll(".CodeMirror-line");
  if (codeMirrorLines && codeMirrorLines.length > 0) {
    return Array.from(codeMirrorLines)
      .map((l) => l.innerText)
      .join("\n");
  }
  return "";
}

function extractSignature(code) {
  if (!code) return "";
  const lines = code.split("\n");
  for (let line of lines) {
    const trimmed = line.trim();
    // Python
    if (trimmed.startsWith("def ")) {
      return trimmed.replace(/:$/, "");
    }
    // C++ / Java / C# / JS / TS / Go / Rust
    if (
      trimmed.includes("(") &&
      !trimmed.startsWith("class ") &&
      !trimmed.startsWith("public class ") &&
      !trimmed.includes("Solution") &&
      !trimmed.startsWith("using ") &&
      !trimmed.startsWith("#") &&
      !trimmed.startsWith("import ") &&
      !trimmed.startsWith("package ")
    ) {
      return trimmed.replace(/\s*\{$/, "");
    }
  }
  // Fallback
  for (let line of lines) {
    const trimmed = line.trim();
    if (trimmed.includes("(")) {
      return trimmed.replace(/\s*\{$/, "");
    }
  }
  return "";
}
