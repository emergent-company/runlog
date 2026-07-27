// HTMX 4 hx-head extension — merge <head> tags (styles, scripts) into partial responses.
// Source: https://unpkg.com/htmx.org@4.0.0-alpha8/dist/ext/hx-head.js
(function () {

    let api

    function appendNode(newNode) {
        let newElt = document.createElement(newNode.tagName)
        for (const attr of newNode.attributes) newElt.setAttribute(attr.name, attr.value)
        newElt.textContent = newNode.textContent

        if (newNode.tagName === "LINK" && newNode.rel === "stylesheet") {
            return new Promise(resolve => {
                newElt.onload = resolve
                newElt.onerror = resolve
                document.head.appendChild(newElt)
            })
        }

        if (newNode.tagName === "SCRIPT" && newNode.src && !newNode.async && !newNode.defer) {
            return new Promise((resolve, reject) => {
                newElt.onload = resolve
                newElt.onerror = reject
                document.head.appendChild(newElt)
            })
        }

        document.head.appendChild(newElt)
        if (newNode._preloadHint) newElt.addEventListener("load", () => newNode._preloadHint.remove(), {once: true})
        return null
    }

    async function mergeHead(newContent, defaultMergeStrategy) {
        if (newContent && newContent.indexOf('<head') > -1) {
            const htmlDoc = document.createElement("html")
            let contentWithSvgsRemoved = newContent.replace(/<svg(\s[^>]*>|>)([\s\S]*?)<\/svg>/gim, '')
            let headTag = contentWithSvgsRemoved.match(/(<head(\s[^>]*>|>)([\s\S]*?)<\/head>)/im)

            if (headTag) {
                let added = []
                let removed = []
                let preserved = []
                let nodesToAppend = []
                let deferred = []

                htmlDoc.innerHTML = headTag
                let newHeadTag = htmlDoc.querySelector("head")
                let currentHead = document.head

                if (newHeadTag == null) return []

                let srcToNewHeadNodes = new Map()
                for (const newHeadChild of newHeadTag.children) {
                    srcToNewHeadNodes.set(newHeadChild.outerHTML, newHeadChild)
                }

                let mergeStrategy = api.attributeValue(newHeadTag, "hx-head") || defaultMergeStrategy

                for (const currentHeadElt of currentHead.children) {
                    let inNewContent = srcToNewHeadNodes.has(currentHeadElt.outerHTML)
                    let isReAppended = currentHeadElt.getAttribute("hx-head") === "re-eval"
                    let isPreserved = api.attributeValue(currentHeadElt, "hx-preserve") === "true"
                    if (inNewContent || isPreserved) {
                        if (isReAppended) {
                            removed.push(currentHeadElt)
                        } else {
                            srcToNewHeadNodes.delete(currentHeadElt.outerHTML)
                            preserved.push(currentHeadElt)
                        }
                    } else {
                        if (mergeStrategy === "append") {
                            if (isReAppended) {
                                removed.push(currentHeadElt)
                                nodesToAppend.push(currentHeadElt)
                            }
                        } else {
                            if (htmx.trigger(document.body, "htmx:before:head:remove", {headElement: currentHeadElt}) !== false) {
                                removed.push(currentHeadElt)
                            }
                        }
                    }
                }

                nodesToAppend.push(...srcToNewHeadNodes.values())

                for (const newNode of nodesToAppend) {
                    if (newNode.tagName === "SCRIPT" && newNode.defer) {
                        deferred.push(newNode)
                        if (newNode.src) {
                            let hint = document.createElement("link")
                            hint.rel = newNode.type === "module" ? "modulepreload" : "preload"
                            hint.as = "script"
                            hint.href = newNode.src
                            document.head.appendChild(hint)
                            newNode._preloadHint = hint
                        }
                    } else {
                        if (htmx.trigger(document.body, "htmx:before:head:add", {headElement: newNode}) !== false) {
                            await appendNode(newNode)
                            added.push(newNode)
                        }
                    }
                }

                for (const removedElement of removed) {
                    if (htmx.trigger(document.body, "htmx:before:head:remove", {headElement: removedElement}) !== false) {
                        currentHead.removeChild(removedElement)
                    }
                }

                htmx.trigger(document.body, "htmx:after:head:merge", {
                    added: added,
                    kept: preserved,
                    removed: removed
                })

                return deferred
            }
        }
        return []
    }

    htmx.registerExtension("hx-head", {
        init: (internalAPI) => {
            api = internalAPI;
        },
        htmx_before_response: (elt, detail) => {
            let ctx = detail.ctx
            let target = ctx.target
            let defaultMergeStrategy = target === document.body ? "merge" : "append";
            if (htmx.trigger(document.body, "htmx:before:head:merge", detail)) {
                let realText = ctx.response.raw.text.bind(ctx.response.raw)
                ctx.response.raw.text = async () => {
                    let text = await realText()
                    ctx._deferredHeadScripts = await mergeHead(text, defaultMergeStrategy)
                    return text
                }
            }
        },
        htmx_after_swap: (elt, detail) => {
            for (const node of detail.ctx._deferredHeadScripts || []) appendNode(node)
        }
    })
})()
