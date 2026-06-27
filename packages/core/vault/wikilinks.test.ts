import { describe, it, expect } from "vitest";
import { transformWikilinks, rewriteEmbeds } from "./wikilinks";

describe("transformWikilinks", () => {
  const resolve = (name: string) => `vault:${name}`;

  it("turns [[Note Name]] into a markdown link", () => {
    expect(transformWikilinks("see [[Note Name]] here", resolve)).toBe(
      "see [Note Name](vault:Note Name) here",
    );
  });

  it("keeps the alias for [[Note|alias]]", () => {
    expect(transformWikilinks("[[Note|the alias]]", resolve)).toBe(
      "[the alias](vault:Note)",
    );
  });

  it("renders an unresolved link as plain text (no broken href)", () => {
    expect(transformWikilinks("[[Ghost]]", () => null)).toBe("Ghost");
  });

  it("leaves embeds (![[...]]) untouched", () => {
    expect(transformWikilinks("![[image.png]]", resolve)).toBe("![[image.png]]");
  });

  it("handles multiple links in one line", () => {
    expect(transformWikilinks("[[A]] and [[B|bee]]", resolve)).toBe(
      "[A](vault:A) and [bee](vault:B)",
    );
  });
});

describe("rewriteEmbeds", () => {
  const toFileUrl = (target: string) => `https://api/file?path=${encodeURIComponent(target)}`;

  it("rewrites ![[img.png]] to a markdown image with the file URL", () => {
    expect(rewriteEmbeds("![[img.png]]", toFileUrl)).toBe(
      "![img.png](https://api/file?path=img.png)",
    );
  });

  it("uses the alias as the image alt for ![[img.png|caption]]", () => {
    expect(rewriteEmbeds("![[folder/img.png|caption]]", toFileUrl)).toBe(
      "![caption](https://api/file?path=folder%2Fimg.png)",
    );
  });

  it("leaves plain wikilinks ([[...]]) untouched", () => {
    expect(rewriteEmbeds("[[Note]]", toFileUrl)).toBe("[[Note]]");
  });

  it("composes with transformWikilinks (embeds first, then links)", () => {
    const md = "![[pic.png]] and [[Note|n]]";
    const out = transformWikilinks(rewriteEmbeds(md, toFileUrl), (n) => `vault:${n}`);
    expect(out).toBe("![pic.png](https://api/file?path=pic.png) and [n](vault:Note)");
  });
});
