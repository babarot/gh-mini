// <Partial name="x" /> as figures/x.part.html next to the file shown, its
// <style> elements taken out and given as the plugin's CSS, which applies
// to what it shows alone.

export default {
  elements: {
    async Partial({ name }, ctx) {
      if (typeof name !== "string" || !/^[\w.-]+$/.test(name)) throw new Error("no name");
      const src = await ctx.read(`figures/${name}.part.html`);
      const css = [];
      const html = src.replace(/<style>([\s\S]*?)<\/style>/g, (_, c) => (css.push(c), ""));
      return { html, css: css.join("\n") };
    },
  },
};
