# Browserarchitectuur

Status: uitgevoerd op 24 juli 2026.

## Uitkomst

De browser is nu opgebouwd als een keten met expliciete eigenaars:

```text
HTTP/TLS ──> Session/DOM ──> cascade + resources
                                │
                                v
                         layout(Viewport)
                                │
                                v
                    Page (DOM-vrij waardemodel)
                                │
                                v
                    View (paint + hit-test)
                                │
                                v
                       Drive (event-loop)
```

De vier oorspronkelijke verzamelbestanden zijn teruggebracht tot hun kern:

- `browse.go`: layouttoestand, het boxmodel en de hoofdwandeling;
- `css.go`: het CSS-datamodel en de gedragen propertygrens;
- `session.go`: navigatie, formulieren en de publieke sessie-API;
- `svg.go`: alleen de eigenlijke SVG-rasterkern.

De complexe delen zijn niet over meer packages verspreid. Ze blijven samen
in `browse`, maar zijn per browserfase in bestanden verdeeld. Daardoor zijn
interne typen en snelle functieaanroepen behouden, zonder dat netwerk,
layout en paint nog door elkaar staan.

## Onderdelen

### App- en presentatielaag

- `drive.go`: event-loop, navigatiewerker, resize en één foutpad voor
  `Present`;
- `view.go`: tekenen, scrollen, browserchrome en hit-testing;
- `Page`, `Box` en `Field`: het complete resultaat dat `View` nodig heeft.

`Page` bevat geen DOM-pointers meer. Een formuliercontrol krijgt een
`ControlID`; alleen `Session` kent de vertaling terug naar `*html.Node`.
Daarmee is de grens layout → paint een gewoon waardemodel.

### Layout

- `browse.go`: hoofdflow en openen/sluiten van een box;
- `layout_element.go`: vroege HTML-semantiek en out-of-flow routes;
- `layout_style.go`: UA-cascade, overerving en tekststijl;
- `layout_replaced.go`: afbeeldingen, video en form-controls;
- `layout_children.go`: gewone, flex- en kolom-child-flow;
- `layout_flow.go`: regels, marges en run-merging;
- `layout_content.go`: tekst, widgets en beeldinhoud;
- `layout_position.go`: absolute, fixed en floats;
- `layout_nodes.go`: DOM- en zichtbaarheidshulpen voor layout;
- `layout_plan.go`: tabel-, grid- en flexplanning;
- `layout_flex.go`: flexmaten en wraprijen;
- `layout_columns.go`: speculatieve cellayout en commit;
- `layout_alignment.go`: justify-, align- en order-logica;
- `layout_svg.go`: inline SVG in de layout.

De kolomcode is langs zijn natuurlijke fasen verdeeld: eerst plannen, dan
meten/wrappen, vervolgens speculatief layouten en pas bij een bruikbare rij
committen. De terugval naar gewone blokflow blijft bij de aanroeper.

### CSS en cascade

- `css.go`: context en gedragen properties;
- `css_parse.go`: stylesheet-, mediaquery- en selectorparsing;
- `css_decls.go`: declaraties, normalisatie, kleuren en variabelen;
- `css_values.go`: relatieve maten, `calc`, boxranden en flex/gridwaarden;
- `cascade.go`: stylesheetladen, selector-matching en computed styles.

`vw`, `vh` en `rem` leven in een `cssContext` per layout. Er is geen
procesglobale viewportstate meer. `LayoutViewport` maakt de hoogte
expliciet; de oude `Layout(width)`-API blijft als compatibele ingang met een
vaste standaardhoogte bestaan.

### Laden en resources

- `loader.go`: HTTP, TLS, cookies, charset en de gedeelde byte-fetcher;
- `resources.go`: afbeeldingselectie, begrenzing, decode en schaal;
- `icon.go`: site-iconen;
- `svg_dom.go`: externe SVG-symbolen in de DOM;
- `svg.go`: SVG-rasterisatie.

Elke `Session` bezit zijn eigen cookie-jar; alleen het concurrency-safe
TLS-transport en zijn connection pool worden gedeeld. Subresources delen
per pagina één URL-cache en gelijktijdige vragers delen één request. De
cache wordt bij navigatie geleegd, zodat een lange browsersessie op bare
metal geen onbegrensde verzameling beelden en stylesheets vasthoudt.

## Vereenvoudigingen buiten browse

- De calculator-controller staat eenmaal in `app/calc/drive.go`; Tamago en
  de host-desktop gebruiken exact hetzelfde pad.
- De drie kopieën van het non-blocking “laatste event wint”-patroon zijn
  vervangen door `ui.PostLatest`.
- `PostLatest` kan niet meer spinnen op een ongebufferd kanaal.
- Screenshots en live websites zijn expliciete meettaken. Gewone tests
  schrijven niets en vereisen geen internet.

## Invarianten

Bij verdere uitbreiding gelden deze grenzen:

1. Netwerk en DOM blijven eigendom van `Session`.
2. Layout krijgt alle omgevingsmaten via `Viewport`/`cssContext`.
3. `Page` blijft DOM-vrij; interactie loopt via stabiele ID's.
4. Alleen `View` schrijft pixels; alleen `Drive` roept `Present` aan.
5. Alle subresources lopen via `fetchResource`, inclusief CSS, SVG en
   iconen.
6. Een nieuw sitegedrag begint als offline fixture in
   `app/browse/testdata/spec`.

## Verificatie

De refactor wordt bewaakt door:

- de volledige offline browser-spec;
- viewporttests met gelijktijdige, verschillende `vw`/`vh`/`rem`-contexten;
- een test voor expliciete viewporthoogte;
- een coalescingtest voor de subresourcecache;
- formulier- en navigatieregressies;
- `go test -race` op browser, UI en calculator;
- `tools/test.sh`: alle hosttests, de desktop-build en alle Tamago- en
  lnetonet-builds.

Visuele artifacts blijven beschikbaar, maar alleen expliciet:

```sh
tools/browser-shots.sh
```
