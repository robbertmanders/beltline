import { useEffect } from "react";

export function NotFoundPage() {
  useEffect(() => { document.title = "Beltline — Not found"; }, []);
  return <article className="page-content"><header className="page-heading"><div><p className="eyebrow">Route</p><h1>Not found</h1></div></header><section className="empty-panel"><h2>This page does not exist</h2><p>Choose a destination from the navigation.</p></section></article>;
}
