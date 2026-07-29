import { Html, Head, Main, NextScript } from "next/document";

const THEME_INIT_SCRIPT = `
(function () {
  try {
    var stored = localStorage.getItem("theme");
    var mode = stored === "light" || stored === "dark" ? stored : null;
    if (mode) {
      document.documentElement.classList.add(mode);
    }
  } catch (e) {}
})();
`;

export default function Document() {
  return (
    <Html lang="en">
      <Head>
        <script dangerouslySetInnerHTML={{ __html: THEME_INIT_SCRIPT }} />
      </Head>
      <body>
        <Main />
        <NextScript />
      </body>
    </Html>
  );
}
