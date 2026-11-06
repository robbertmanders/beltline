import { expect, type Page } from "@playwright/test";

/** The width of every stage cell in the belts editor, the trigger included (belts.css). */
const stageCardWidth = 150;
/** The belts cases whose stage line is measured after they are ready, before capture. */
export const stageLineCases = ["belts-trigger-reviews", "belts-stage-form", "belts-nested-forks", "belts-epic"];
/** The stage forms whose cards and model rows are checked: the editor scrolls to each card, then goes back before capture. */
export const stageFormCases = ["belts-agent-stage-form", "belts-model-pool", "belts-codex-stage", "belts-model-error", "belts-stage-form", "belts-watch-stage", "belts-route-stage"];

/** Every stage cell on the editor's line is 150 px wide, the trigger included, offscreen cards too. */
async function expectStageCardWidths(page: Page, caseId: string): Promise<void> {
  const cards = await page.locator(".stage-line-card .stage-block").evaluateAll(elements => elements.map(element => ({
    name: element.getAttribute("aria-label") ?? "",
    width: element.getBoundingClientRect().width
  })));
  expect(cards.length, `${caseId} should draw stage cards`).toBeGreaterThan(0);
  for (const card of cards) {
    expect(Math.abs(card.width - stageCardWidth), `${caseId} card ${card.name} should be ${stageCardWidth}px wide`).toBeLessThanOrEqual(0.5);
  }
}

/** A short main line starts at the row's left edge: the trigger, then each stage after its 26 px connector, with nothing between them. */
async function expectJoinedLine(page: Page, caseId: string): Promise<void> {
  const flow = await page.locator(".stage-line-row").first().evaluate(row => {
    const boxes: Array<{ kind: string; left: number; right: number; width: number }> = [];
    const add = (kind: string, element: Element) => {
      const rect = element.getBoundingClientRect();
      boxes.push({ kind, left: rect.left, right: rect.right, width: rect.width });
    };
    for (const child of [...row.children]) {
      if (!child.classList.contains("stage-line-piece")) {
        add(child.classList.contains("stage-block-trigger") ? "trigger" : "connector", child);
      } else if (child.querySelector(":scope > .stage-block-wrap")) {
        add("connector", child.querySelector(":scope > .add-menu")!);
        add("stage", child.querySelector(":scope > .stage-block-wrap")!);
      } else {
        add("fork", child);
      }
    }
    return { rowLeft: row.getBoundingClientRect().left, boxes };
  });
  expect(flow.boxes[0]?.kind, `${caseId} should start with the trigger`).toBe("trigger");
  expect(Math.abs(flow.boxes[0]!.left - flow.rowLeft), `${caseId} trigger should start at the row's left edge`).toBeLessThanOrEqual(0.5);
  for (let index = 1; index < flow.boxes.length; index += 1) {
    const previous = flow.boxes[index - 1]!;
    const current = flow.boxes[index]!;
    expect(Math.abs(current.left - previous.right), `${caseId} ${current.kind} ${index} should immediately follow the previous piece`).toBeLessThanOrEqual(0.5);
    if (current.kind === "stage") expect(previous.kind, `${caseId} stage ${index} should follow its connector`).toBe("connector");
    if (current.kind === "connector") expect(Math.abs(current.width - 26), `${caseId} connector ${index} should be 26px wide`).toBeLessThanOrEqual(0.5);
  }
}

/** A belt wider than the stage area scrolls inside it: the far end holds the last main-line card, 150 px wide, and the scroll position is restored. */
async function expectScrollToEnd(page: Page, caseId: string): Promise<void> {
  const scroll = page.locator(".stage-line-scroll").first();
  const before = await scroll.evaluate(element => ({ overflows: element.scrollWidth > element.clientWidth, left: element.scrollLeft }));
  expect(before.overflows, `${caseId} should overflow horizontally at this size`).toBe(true);
  await scroll.evaluate(element => { element.scrollLeft = element.scrollWidth; });
  const last = await page.locator(".stage-line-row > .stage-line-piece > .stage-block-wrap .stage-block").last().evaluate(card => {
    const view = card.closest(".stage-line-scroll")!.getBoundingClientRect();
    const rect = card.getBoundingClientRect();
    return { width: rect.width, insideLeft: rect.left >= view.left - 0.5, insideRight: rect.right <= view.right + 0.5 };
  });
  await scroll.evaluate((element, left) => { element.scrollLeft = left; }, before.left);
  expect(Math.abs(last.width - stageCardWidth), `${caseId} final main-line card should be ${stageCardWidth}px wide`).toBeLessThanOrEqual(0.5);
  expect(last.insideLeft && last.insideRight, `${caseId} final main-line card should be within the stage area`).toBe(true);
}

/** The long card's title ellipsises inside it, its full name is in its accessible name and hover title, and the stage form shows it in full. */
async function expectLongCardName(page: Page, caseId: string): Promise<void> {
  const name = "Open the feature branch";
  const card = page.locator(`.stage-line-card .stage-block[aria-label="${name}"]`);
  await expect(card, `${caseId} should draw the long card once`).toHaveCount(1);
  const measured = await card.evaluate(element => {
    const cardRect = element.getBoundingClientRect();
    const inside = (rect: DOMRect) => rect.left >= cardRect.left - 0.5 && rect.right <= cardRect.right + 0.5;
    const title = element.querySelector<HTMLElement>(".stage-block-title")!;
    const titleStyle = getComputedStyle(title);
    const titleRect = title.getBoundingClientRect();
    const lines = [...element.querySelectorAll<HTMLElement>(".stage-block-lines > span")].map(line => {
      const style = getComputedStyle(line);
      return { inside: inside(line.getBoundingClientRect()), ellipsis: style.textOverflow === "ellipsis" && style.whiteSpace === "nowrap" && style.overflow === "hidden" };
    });
    return {
      width: cardRect.width,
      titleOverflows: title.scrollWidth > title.clientWidth,
      titleInside: inside(titleRect),
      titleOneLine: titleRect.height <= parseFloat(titleStyle.lineHeight) + 0.5,
      titleEllipsis: titleStyle.textOverflow === "ellipsis" && titleStyle.whiteSpace === "nowrap",
      titleText: title.textContent ?? "",
      lines
    };
  });
  expect(Math.abs(measured.width - stageCardWidth), `${caseId} long card should be ${stageCardWidth}px wide`).toBeLessThanOrEqual(0.5);
  expect(measured.titleOverflows, `${caseId} long card title should overflow its box`).toBe(true);
  expect(measured.titleEllipsis, `${caseId} long card title should use a single-line ellipsis`).toBe(true);
  expect(measured.titleOneLine, `${caseId} long card title should stay on one line`).toBe(true);
  expect(measured.titleInside, `${caseId} long card title should stay inside the card`).toBe(true);
  expect(measured.titleText, `${caseId} long card title should keep its full name`).toBe(name);
  for (const [index, line] of measured.lines.entries()) {
    expect(line.inside, `${caseId} summary line ${index + 1} should stay inside the card`).toBe(true);
    expect(line.ellipsis, `${caseId} summary line ${index + 1} should use the ellipsis styling`).toBe(true);
  }
  await expect(card).toHaveAccessibleName(new RegExp(name));
  expect(await card.getAttribute("title"), `${caseId} hover title should carry the full name`).toContain(name);
  await card.click();
  const form = page.locator('.stage-form[data-ready="true"]');
  await expect(form, `${caseId} should open the stage form`).toBeVisible();
  await expect(form.getByLabel("Name", { exact: true }), `${caseId} stage form should show the full name`).toHaveValue(name);
  const selected = await page.locator(".stage-line-card .stage-block.is-selected").evaluate(element => element.getBoundingClientRect().width);
  expect(Math.abs(selected - stageCardWidth), `${caseId} selected long card should stay ${stageCardWidth}px wide`).toBeLessThanOrEqual(0.5);
  await page.locator(".stage-line-card .stage-block-trigger").click();
  await expect(form, `${caseId} should close the stage form when Trigger is selected again`).toHaveCount(0);
}

/**
 * A stage form's cards can each be brought into view in the belt editor's scroll area, its outcome table scrolls
 * inside a viewport that fits the form's column, and the editor's scroll position is put back before capture.
 * An agent stage has the Brief card; an approval or watch has none.
 */
export async function expectStageCardsInView(page: Page, caseId: string): Promise<void> {
  const scroller = page.locator(".belts-main").first();
  const form = page.locator(".stage-form[data-ready='true']").first();
  const before = await scroller.evaluate(element => element.scrollTop);
  const hasBrief = (await page.locator(".stage-form > section[aria-label='Stage brief']").count()) > 0;
  const targets = [
    ...(hasBrief ? [".stage-form > section[aria-label='Stage brief']"] : []),
    ".stage-form > section[aria-label='Stage outcomes']",
    ".stage-form .outcome-preview"
  ];
  for (const selector of targets) {
    const target = page.locator(selector).first();
    await target.evaluate(element => element.scrollIntoView({ block: "start" }));
    const placed = await target.evaluate(element => {
      const view = element.closest(".belts-main")!.getBoundingClientRect();
      const rect = element.getBoundingClientRect();
      return { top: rect.top, visibleTop: rect.top >= view.top - 0.5 && rect.top < view.bottom, height: rect.height };
    });
    expect(placed.height, `${caseId} ${selector} should have height`).toBeGreaterThan(0);
    expect(placed.visibleTop, `${caseId} ${selector} should come into view in the editor`).toBe(true);
  }
  const fits = await page.locator(".stage-form .outcomes-scroll").first().evaluate(element => {
    const form = element.closest(".stage-form")!.getBoundingClientRect();
    return element.getBoundingClientRect().right <= form.right + 0.5 && element.clientWidth > 0;
  });
  expect(fits, `${caseId} outcome table viewport should fit the stage form's column`).toBe(true);
  await scroller.evaluate((element, top) => { element.scrollTop = top; }, before);
  expect(await scroller.evaluate(element => element.scrollTop), `${caseId} should restore the editor scroll position`).toBe(before);
  await expect(form, `${caseId} should still show its stage form`).toBeVisible();
}

/** The stage line checks for a belts case, run once it is ready and before its overflow check and capture. */
export async function assertStageLine(page: Page, caseId: string, size: { width: number; height: number }): Promise<void> {
  await expectStageCardWidths(page, caseId);
  if (caseId === "belts-stage-form") await expect(page.locator(".stage-line-card .stage-block.is-selected"), `${caseId} should select one card`).toHaveCount(1);
  if (caseId === "belts-trigger-reviews") await expectJoinedLine(page, caseId);
  if (caseId === "belts-nested-forks" && size.width === 1080) await expectScrollToEnd(page, caseId);
  if (caseId === "belts-epic") await expectLongCardName(page, caseId);
}
