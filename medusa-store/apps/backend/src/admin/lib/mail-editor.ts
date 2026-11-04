// Keep pasted mail markup inert before mounting it inside the Admin document.
export function editableMailHtml(value: string) {
  const document = new DOMParser().parseFromString(value, "text/html")
  document
    .querySelectorAll(
      "script,style,link,meta,base,iframe,object,embed,svg,math,form,input,button,textarea,select,video,audio,source"
    )
    .forEach((node) => node.remove())
  const tags = new Set(
    "p div span br b strong i em u s strike a img ul ol li blockquote pre code h1 h2 h3 h4 h5 h6 table caption thead tbody tfoot tr th td hr font sub sup".split(
      " "
    )
  )
  const attributes = new Set(
    "href src alt title width height colspan rowspan align color face size style".split(
      " "
    )
  )
  const styles = new Set(
    "color background-color font-family font-size font-weight font-style text-decoration text-align vertical-align line-height width height max-width border border-width border-style border-color border-collapse padding margin margin-left margin-right".split(
      " "
    )
  )
  Array.from(document.body.querySelectorAll("*")).forEach((node) => {
    if (!tags.has(node.tagName.toLowerCase())) {
      node.replaceWith(...Array.from(node.childNodes))
      return
    }
    Array.from(node.attributes).forEach(({ name, value }) => {
      if (
        !attributes.has(name) ||
        ((name === "href" || name === "src") &&
          !safeMailUrl(value, name === "src"))
      )
        node.removeAttribute(name)
    })
    const element = node as HTMLElement
    if (element.hasAttribute("style")) {
      const original = element.style
      const clean = document.createElement("span").style
      Array.from(original).forEach((property) => {
        const value = original.getPropertyValue(property)
        if (
          (styles.has(property) ||
            /^(border-(top|right|bottom|left)(-(width|style|color))?|padding-(top|right|bottom|left)|margin-(top|right|bottom|left))$/.test(
              property
            )) &&
          !/url\s*\(|expression|@import|[\\<>]/i.test(value)
        )
          clean.setProperty(property, value)
      })
      element.setAttribute("style", clean.cssText)
    }
  })
  return document.body.innerHTML
}

export function safeMailUrl(value: string, image = false) {
  try {
    const url = new URL(value)
    return (
      !url.username &&
      !url.password &&
      (image
        ? url.protocol === "https:"
        : ["https:", "http:", "mailto:", "tel:"].includes(url.protocol))
    )
  } catch {
    return false
  }
}

export function mailTextHtml(text: string) {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\n/g, "<br>")
}

export function mailHtmlText(html: string) {
  const document = new DOMParser().parseFromString(
    editableMailHtml(html),
    "text/html"
  )
  document.querySelectorAll("br").forEach((node) => node.replaceWith("\n"))
  document
    .querySelectorAll("p,div,li,blockquote,pre,h1,h2,h3,h4,h5,h6,tr")
    .forEach((node) => {
      node.prepend("\n")
      node.append("\n")
    })
  document.querySelectorAll("td,th").forEach((node) => node.append("\t"))
  document.querySelectorAll("img").forEach((node) => node.replaceWith(node.alt))
  return (document.body.textContent || "")
    .replace(/\n{3,}/g, "\n\n")
    .replace(/^\n/, "")
    .trimEnd()
}
