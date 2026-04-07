# Motivation, decisions, and current state

See also: [positioning.md](positioning.md) — analysis of how godom is perceived vs what it actually is, and action items to clarify.



> What problem godom solves for me

I am used to writing a lot of Go code or Python code for analysis or experimentation.
After a certain point, the built-in stuff or Python notebooks feel more hacky.
The terminal-based outputs or reports stop fulfilling the purpose and are not easy to look at.
So I end up generating HTML outputs. Mostly from Go.
Once the outputs become larger, the HTML which is already large becomes tedious to manage. Interactivity, while possible, becomes tedious to add or change. Naturally this causes friction.
When it becomes complex enough, then comes this whole SPA ecosystem and API exposures from local. SPAs later become large, and if I choose to get into the npm world - it usually ends up becoming code that will largely get outdated once the experiment ends. 6 months or a year later - if I revisit an experiment - things have gone off or there's a security bug. I am personally more afraid of npm than phishing links.
What should have been a cool experiment turns into friction once it grows - with nobody else to maintain it.

Godom was born partly out of that necessity to do things in a certain way. And of course Godom too could become an experiment that I might regret later, however I have put enough effort that I do not have to revisit "framework readiness" for basic features.

But if I am going to consider this as a web app framework, I might be disappointed. Because the whole idea and effort revolved around local apps on browser, not web apps.

In the journey I have found some interesting use cases.
- I was not thinking of supporting it over network, but then it was a simple thing to do with minimal effort. It just made things simpler to use the app.
- I was not planning to have multi-window or multi-machine sync. But that is a side-effect that is very useful in some cases. I should have more examples around it.
    - For example - the breakout game uses the mobile as a controller for a game that is played on another large screen, while being served from a third machine. All that just using a Wi-Fi network.
    - Another use case is having a mobile as a controller via godom sending commands to a real car racing app. And it's Go, so it can send virtual keystrokes to the game.
    - Or the use case of sharing the iPad pen strokes to a screen shown in a meeting or a recording.



> Why multipage and routing

Sometimes we don't want all components shown on a view. So it makes no sense to load those up on the browser - because components that are loaded will receive updates from their VDOM on the server. If a component is not shown, those updates will still be received, but ignored, reducing browser processing.

A godom component is self-contained. It has its own VDOM (view-state) and it will sync across multiple real views - like multiple browsers or multiple windows or multiple pages.

I still do not like the idea that the VDOM component is loaded in Go regardless of the browser using it - but that is a problem which creates conflict - not about "how to solve it", but more about "should it be solved" and "is that really a problem". For now, I have not found a use case which says "it must be solved by the framework."

Multi-pages allow me to scope the browser-side views. And this has helped me integrate multiple tools under a single large Go program.

Godom also supports multiple engines which can run on multiple ports or can be a different Go process too. While this is supported, it needs more testing and examples. The bridge can be namespaced, but included godom.js files from multiple godom-served apps need better management from the bridge. Currently it will need workarounds on the browser JS side.



> My attempts to build a real multi-user webapp with Godom

Even though it started as a tool for local apps, I did get tempted to build a webapp.

There was a particular SaaS app that actually made more sense integrating with Godom.

However Godom uses WebSockets as of now, and that is not what I wanted. And that could be solved, however the concept of server-managed VDOM/view-state was another problem. That could be solved too... but it is a step backwards and I would be making a webapp framework again — plenty of good ones exist.

There is a way where it is possible to use the godom templating and VDOM tree outputs to generate HTML. I have not explored it in detail yet. It means more framework changes.



> Current state

I ran out of time I allotted for godom development, it took much longer than the initial idea. Multi-page support and injectable support was the last thing I needed for it to work with my use cases. Beyond that it delves into the larger internet-based webapps.

I have a lot of examples built showing its capabilities. At this stage I am prioritising building more apps with it and using godom for what it was built for - local apps. Typically integrating the old Go stuff all in a single monolith app (or few large ones).

And, so far, I have been pleasantly surprised at what and how fast it enables me to build stuff that I need without overloading my brain with details of API routes and syncing with the backend.
It's the simple concept - sync the VDOM (godom/view-state) with the browser DOM.
And then everything feels like one single app.

Limitations - we will find out and fix as we move forward.

But when I tried to push it to multi-user SaaS webapps, of course it was disappointing. It has limitations and the godom concept feels less practical for internet-based apps. Godom was built with local-only webapps in mind, typically under trusted environments. Godom-based stuff will work over the internet, but you may not want to. But if godom apps were like game apps that don't run off a single server, maybe that is where godom could shine better. It's just another use case.
