# AI Usage Philosophy

GODOM was built unusually fast.

What started as an experiment quickly turned into a real framework: Go owns the application logic and state, a thin JavaScript bridge connects Go to the browser DOM, and the browser acts primarily as a rendering surface. What I expected to be a small prototype grew into something much more capable in just a few days. It went beyond simple forms and UI controls into richer examples, including graphics and 3D-style animation.

A large reason this happened is AI. This document was also edited with heavy AI assistance.

This project was built with heavy AI assistance, and I want to be explicit about that from the beginning.

## What this means in practice

The idea, architecture, constraints, and direction of GODOM are mine.

But a large portion of the implementation was written by AI.

I did not use AI as a minor assistant or autocomplete tool. I used it as a high-speed implementation partner. I pushed it hard, redirected it when needed, rejected wrong turns, and kept it aligned with the architecture I wanted: Go in control, JavaScript minimized, browser as UI surface, and a simple bridge between the two.

So this is not a case of “AI independently created the project.”  
But it is also not honest to describe it as traditionally hand-written software. A substantial amount of the code was produced by AI under my direction.

## The review philosophy on this project

My earlier philosophy with AI-generated code was simple: review it carefully before treating it as real.

GODOM changed that.

Here, the speed of generation was much faster than the speed at which I could carefully review every line. If I had tried to fully review everything as it was being generated, development would have slowed down so much that the project likely would not have expanded the way it did.

So the working philosophy became something more like this:

- define the architecture
- set constraints
- let AI implement
- functionally test what it produced
- verify that it behaves correctly at a practical level
- keep moving

That means a lot of this project was accepted based on functional behavior and architectural fit, not on exhaustive line-by-line review.

## I may not review all of this code

I want to be direct about something important:

I do not know whether this codebase will ever receive a full exhaustive human review.

It might.
It might not.

I may review parts of it if I use them in a more serious or production context. I may review core areas over time. But I do not want to imply that a complete manual audit is definitely coming, because I am not sure that it is.

This project is being published anyway.

That is not because I think review does not matter. It is because I think the project is already interesting, already useful, and already worth sharing, even in this state.

## Why I am still willing to publish it

Because the result is real.

The project works. The architecture is meaningful. The examples demonstrate that the model is viable. And the speed at which this was discovered is itself part of the story.

If I waited for exhaustive certainty, I would probably not publish it now.
If I insisted on fully reviewing every generated line before sharing anything, I might have stopped the project much earlier.

So I am choosing a different tradeoff:

publish the work,
state clearly how it was made,
state clearly what has and has not been reviewed,
and let others evaluate it on those terms.

## Trust in the model matters here

Part of this philosophy also comes from the fact that I built much of this with Opus 4.6, which has, in my experience, shown strong reasoning ability over the past month.

That matters.

If I had been using a weaker model, I would likely have been more cautious. I may have slowed down more. I may have trusted less. But with Opus 4.6, I developed a meaningful level of trust in its ability to reason through implementation details and stay aligned when properly directed.

That does not mean I believe it is perfect.
It does not mean I think it cannot introduce bugs, poor abstractions, or security issues.
It does mean that I trust it enough that I was willing to move much faster than I normally would.

So this project reflects not just trust in AI in general, but trust in a particular model, in a particular workflow, under heavy human steering.

## Functional trust is not the same as deep audit

Most of my validation here has been functional.

I tested whether things work.
I checked whether the architecture was being respected.
I verified outputs.
I used the framework and examples in practice.

But that is not the same as saying:
- every line has been reviewed,
- every edge case has been explored,
- every security implication has been audited,
- or every unit test is deeply trustworthy.

I am especially cautious about AI-generated tests. AI can write tests that pass without really proving much. It can also reshape tests around its own assumptions. So while tests can still be useful, I do not automatically treat them as strong proof of correctness.

## Security: context matters

GODOM is not intended as a general multi-user web framework.

Its intended use is primarily local UI. That matters for risk.

This project is much closer to “build a local app UI with Go driving the browser” than to “deploy a public multi-user internet-facing application framework.” For local UI work, I do not apply the same security expectations that I would apply to a public network service or a framework meant for shared hostile environments.

So the risk profile is different.

That said, “local” is not the same as “risk-free.”

Some examples may have much more access than others. The terminal example (`examples/terminal/`) gives full shell access to the host machine — it is equivalent to an unlocked terminal session. That example includes its own security documentation (see `examples/terminal/implementation.md`), but if you are running code like that, you should understand exactly what it does and what permissions or capabilities it implies.

So my practical view is:

- for ordinary local UI experimentation, I am less worried
- for anything with system access, terminal access, or network exposure, caution increases significantly
- for any production or serious deployment use, review becomes your responsibility


## If you use this project

If you use GODOM, especially beyond experimentation, you should assume the following:

- the architecture is intentional
- much of the implementation is AI-generated
- not all of it has been deeply reviewed by me
- some examples may be more trustworthy than others
- anything with broader system access deserves extra care
- if you plan serious use, you should review the code yourself

In other words: use it with understanding, not blind trust.

## Authorship and credit

I want to give credit honestly. Steering was real work. It was often the hardest part. At the same time, I do not want to understate AI’s contribution. A large amount of code was written by AI, and the speed and breadth of the project came from that.