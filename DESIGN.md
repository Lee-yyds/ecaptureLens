# eCaptureLens interface

Dense traffic workspace following the user’s Reqable reference: device navigation, searchable request rows and persistent inspector. Material 3 on Android, adaptive split panes in PC browsers. No illustrative traffic in production.

Color: light surface #FCFCFC; green seed #176B4B, semantic error color from Material. Dark scheme derived from #66DBA7 and system theme. Flat separators distinguish panes; avoid ornamental cards. Typography uses the platform Material scale; monospace only for actual headers, payloads and API data.

Desktop: 208px device sidebar at 1180px+, rows 46px, toolbar 12px padding, inspector 39% (340–620px). Below 840px list/details become separate routes. Android respects SafeArea and system Back, touch actions use Material controls with 48dp targets. Text remains selectable and payloads can be copied/exported.

States: empty history calls for device configuration or import; capture commands and API failures remain visible; request without response says Pending; local-library startup errors offer explicit remote connection. History limit is labeled as latest 1,000 requests. Live refresh can be paused independently of capture.
