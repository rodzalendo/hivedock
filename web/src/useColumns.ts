import { useEffect, useState } from "react";

// Tailwind's breakpoints, mirrored here because effectiveColumns replaces the
// responsive grid classes that used to encode them.
const SM = 640;
const LG = 1024;
const XL = 1280;

// effectiveColumns is how many dashboard group columns actually fit at this
// viewport width: the user's preference capped by what the screen can show.
//
// The dashboard buckets groups into columns in JS and renders them with a
// grid-cols-N class, so the two counts MUST agree. When responsive classes
// narrowed the grid on their own (a 4-column layout falling back to
// lg:grid-cols-3), the surplus bucket wrapped onto a second grid row and its
// group appeared far below its neighbours, under the tallest column.
export function effectiveColumns(want: number, width: number): number {
  const wanted = Math.min(4, Math.max(1, want));
  if (wanted === 1 || width < SM) return 1;
  if (wanted === 2) return 2;
  if (width < LG) return 2;
  if (wanted === 3) return width >= XL ? 3 : 2;
  return width >= XL ? 4 : 3;
}

// useEffectiveColumns tracks the viewport so the layout re-buckets on resize.
export function useEffectiveColumns(want: number): number {
  const [width, setWidth] = useState(() =>
    typeof window === "undefined" ? XL : window.innerWidth,
  );
  useEffect(() => {
    const onResize = () => setWidth(window.innerWidth);
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);
  return effectiveColumns(want, width);
}
