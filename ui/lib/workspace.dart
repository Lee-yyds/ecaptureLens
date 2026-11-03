import 'dart:async';
import 'dart:convert';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';

import 'api.dart';
import 'files.dart';
import 'inspector.dart';
import 'settings.dart';

class Workspace extends StatefulWidget {
  const Workspace({super.key, required this.api, this.onDisconnect});
  final LensApi api;
  final VoidCallback? onDisconnect;

  @override
  State<Workspace> createState() => _WorkspaceState();
}

class _WorkspaceState extends State<Workspace> {
  List<Json> _requests = [];
  List<Json> _devices = [];
  List<Json> _statuses = [];
  List<Json> _decoders = [];
  Json? _selected;
  String? _device;
  String? _error;
  bool _refreshing = false;
  bool _action = false;
  bool _live = true;
  int _total = 0;
  final _search = TextEditingController();
  Timer? _timer;
  Timer? _debounce;

  @override
  void initState() {
    super.initState();
    _refresh();
    _timer = Timer.periodic(const Duration(seconds: 1), (_) {
      if (_live) _refresh();
    });
  }

  Future<void> _refresh() async {
    if (_refreshing) return;
    _refreshing = true;
    try {
      final results = await Future.wait([
        widget.api.call('requests.list', {
          'query': _search.text,
          if (_device != null) 'device_id': _device,
          'limit': 1000,
        }),
        widget.api.call('devices.list'),
        widget.api.call('capture.status'),
        widget.api.call('decoders.list'),
      ]);
      if (!mounted) return;
      final list = results[0] as Json;
      final items = (list['items'] as List).cast<Json>();
      final devices = (results[1] as List).cast<Json>();
      setState(() {
        _requests = items;
        _total = list['total'] as int;
        _devices = devices;
        if (!_devices.any((device) => device['id'] == _device)) _device = null;
        _statuses = (results[2] as List).cast<Json>();
        _decoders = (results[3] as List).cast<Json>();
        _error = null;
        if (_selected != null) {
          final matches = items.where((item) => item['id'] == _selected!['id']);
          _selected = matches.isEmpty ? null : matches.first;
        }
      });
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    } finally {
      _refreshing = false;
    }
  }

  Future<void> _run(Future<void> Function() action) async {
    setState(() => _action = true);
    try {
      await action();
      await _refresh();
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    } finally {
      if (mounted) setState(() => _action = false);
    }
  }

  Future<void> _capture(bool start) => _run(() async {
    await widget.api.call(start ? 'capture.start' : 'capture.stop', {
      'device_id': _device,
    });
  });

  Future<void> _export(String format, {bool selected = false}) =>
      _run(() async {
        final result =
            await widget.api.call('history.export', {
                  'format': format,
                  if (selected && _selected != null) 'ids': [_selected!['id']],
                })
                as Json;
        await saveBytes(
          'ecapturelens.$format',
          utf8.encode(result['content'] as String),
        );
      });

  Future<void> _import() => _run(() async {
    final picked = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: ['json', 'har'],
      withData: true,
    );
    if (picked == null) return;
    final file = picked.files.single;
    final bytes = file.bytes!;
    await widget.api.call('history.import', {
      'format': file.extension == 'har' ? 'har' : 'json',
      'content': utf8.decode(bytes),
    });
  });

  Future<void> _settings([String section = 'devices']) async {
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => SettingsPage(api: widget.api, initialSection: section),
      ),
    );
    await _refresh();
  }

  Future<void> _sessions() async {
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) =>
            SessionsPage(api: widget.api, device: _device, decoders: _decoders),
      ),
    );
  }

  void _select(Json item, bool compact) {
    if (compact) {
      Navigator.of(context).push(
        MaterialPageRoute<void>(
          builder: (_) => RequestDetailPage(
            api: widget.api,
            initial: item,
            decoders: _decoders,
          ),
        ),
      );
    }
    setState(() => _selected = item);
  }

  @override
  void dispose() {
    _timer?.cancel();
    _debounce?.cancel();
    _search.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final compact = constraints.maxWidth < 840;
      final sidebar = constraints.maxWidth >= 1180;
      final currentStatuses = _statuses.where((s) => s['device_id'] == _device);
      final state = currentStatuses.isEmpty
          ? 'stopped'
          : currentStatuses.first['state'] as String;
      final capturing =
          state == 'running' || state == 'connected' || state == 'starting';
      return Scaffold(
        appBar: AppBar(
          title: Row(
            children: [
              Icon(
                Icons.center_focus_strong,
                color: Theme.of(context).colorScheme.primary,
              ),
              const SizedBox(width: 10),
              const Expanded(
                child: Text(
                  'eCaptureLens',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
          actions: [
            IconButton(
              onPressed: _sessions,
              icon: const Icon(Icons.timeline),
              tooltip: 'Sessions & timeline',
            ),
            IconButton(
              onPressed: () => _settings(),
              icon: const Icon(Icons.settings_outlined),
              tooltip: 'Devices & settings',
            ),
            if (widget.onDisconnect != null)
              IconButton(
                onPressed: widget.onDisconnect,
                icon: const Icon(Icons.link_off),
                tooltip: 'Change service',
              ),
            const SizedBox(width: 8),
          ],
        ),
        body: SafeArea(
          top: false,
          child: Column(
            children: [
              const Divider(height: 1),
              _toolbar(compact, capturing, state),
              if (currentStatuses.isNotEmpty &&
                  (currentStatuses.first['error'] as String?)?.isNotEmpty ==
                      true)
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                  child: SelectableText(
                    'Capture: ${currentStatuses.first['error']}',
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.error,
                    ),
                  ),
                ),
              if (_error != null)
                MaterialBanner(
                  content: Text('Refresh or operation failed: $_error'),
                  leading: const Icon(Icons.error_outline),
                  actions: [
                    TextButton(onPressed: _refresh, child: const Text('Retry')),
                  ],
                ),
              Expanded(
                child: Row(
                  children: [
                    if (sidebar) SizedBox(width: 208, child: _sidebar()),
                    if (sidebar) const VerticalDivider(width: 1),
                    Expanded(
                      child: Column(
                        children: [
                          if (!compact) _tableHeader(),
                          Expanded(
                            child: _requests.isEmpty
                                ? _empty()
                                : ListView.builder(
                                    itemCount: _requests.length,
                                    itemBuilder: (context, i) =>
                                        _requestRow(_requests[i], compact),
                                  ),
                          ),
                          _statusbar(),
                        ],
                      ),
                    ),
                    if (!compact) ...[
                      const VerticalDivider(width: 1),
                      SizedBox(
                        width: (constraints.maxWidth * .39).clamp(340, 620),
                        child: _selected == null
                            ? const Center(
                                child: Text('Select a request to inspect'),
                              )
                            : Inspector(
                                api: widget.api,
                                transaction: _selected!,
                                decoders: _decoders,
                              ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ),
      );
    },
  );

  Widget _toolbar(bool compact, bool capturing, String state) {
    final button = FilledButton.icon(
      onPressed: _action || _device == null ? null : () => _capture(!capturing),
      icon: Icon(capturing ? Icons.stop : Icons.fiber_manual_record, size: 17),
      label: Text(capturing ? 'Stop' : 'Capture'),
    );
    final device = DropdownButtonFormField<String>(
      key: ValueKey(_device),
      initialValue: _device,
      isExpanded: true,
      hint: const Text('Select device'),
      decoration: const InputDecoration(
        prefixIcon: Icon(Icons.smartphone_outlined),
        contentPadding: EdgeInsets.symmetric(horizontal: 12, vertical: 12),
      ),
      items: _devices
          .map(
            (d) => DropdownMenuItem(
              value: d['id'] as String,
              child: Text(
                d['name'] as String? ?? d['id'] as String,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          )
          .toList(),
      onChanged: (value) {
        setState(() => _device = value);
        _refresh();
      },
    );
    final search = TextField(
      controller: _search,
      decoration: const InputDecoration(
        hintText: 'Filter URL, method, status or content',
        prefixIcon: Icon(Icons.search),
      ),
      onChanged: (_) {
        _debounce?.cancel();
        _debounce = Timer(const Duration(milliseconds: 250), _refresh);
      },
    );
    final menu = PopupMenuButton<String>(
      tooltip: 'Traffic actions',
      onSelected: (value) async {
        switch (value) {
          case 'json':
          case 'har':
            await _export(value);
          case 'import':
            await _import();
          case 'refresh':
            await _refresh();
          case 'clear':
            final clear = await showDialog<bool>(
              context: context,
              builder: (context) => AlertDialog(
                title: const Text('Clear request history?'),
                content: const Text(
                  'This removes all 1,000 retained requests. Export anything you need first.',
                ),
                actions: [
                  TextButton(
                    onPressed: () => Navigator.pop(context, false),
                    child: const Text('Cancel'),
                  ),
                  FilledButton(
                    onPressed: () => Navigator.pop(context, true),
                    child: const Text('Clear history'),
                  ),
                ],
              ),
            );
            if (clear == true) {
              await _run(() async {
                await widget.api.call('requests.clear');
              });
            }
        }
      },
      itemBuilder: (_) => const [
        PopupMenuItem(value: 'import', child: Text('Import JSON / HAR')),
        PopupMenuItem(value: 'json', child: Text('Export JSON')),
        PopupMenuItem(value: 'har', child: Text('Export HAR')),
        PopupMenuItem(value: 'refresh', child: Text('Refresh now')),
        PopupMenuDivider(),
        PopupMenuItem(value: 'clear', child: Text('Clear history…')),
      ],
    );
    return Padding(
      padding: const EdgeInsets.all(12),
      child: compact
          ? Column(
              children: [
                Row(
                  children: [
                    Expanded(child: device),
                    const SizedBox(width: 12),
                    button,
                    menu,
                  ],
                ),
                const SizedBox(height: 12),
                search,
              ],
            )
          : Row(
              children: [
                SizedBox(width: 220, child: device),
                const SizedBox(width: 12),
                button,
                const SizedBox(width: 18),
                Expanded(child: search),
                const SizedBox(width: 8),
                IconButton(
                  onPressed: () => setState(() => _live = !_live),
                  tooltip: _live ? 'Pause live refresh' : 'Resume live refresh',
                  icon: Icon(
                    _live
                        ? Icons.pause_circle_outline
                        : Icons.play_circle_outline,
                  ),
                ),
                menu,
              ],
            ),
    );
  }

  Widget _sidebar() => ListView(
    padding: const EdgeInsets.symmetric(vertical: 20),
    children: [
      Padding(
        padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
        child: Text('Workspace', style: Theme.of(context).textTheme.titleSmall),
      ),
      ListTile(
        selected: _device == null,
        leading: const Icon(Icons.all_inbox_outlined),
        title: const Text('All traffic'),
        onTap: () {
          setState(() => _device = null);
          _refresh();
        },
      ),
      const Divider(indent: 16, endIndent: 16),
      for (final d in _devices)
        ListTile(
          selected: _device == d['id'],
          leading: const Icon(Icons.smartphone_outlined),
          title: Text(
            d['name'] as String? ?? d['id'] as String,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
          onTap: () {
            setState(() => _device = d['id'] as String);
            _refresh();
          },
        ),
      ListTile(
        leading: const Icon(Icons.add),
        title: const Text('Add device'),
        onTap: () => _settings(),
      ),
      const Divider(indent: 16, endIndent: 16),
      ListTile(
        leading: const Icon(Icons.timeline),
        title: const Text('Sessions'),
        onTap: _sessions,
      ),
      ListTile(
        leading: const Icon(Icons.data_object),
        title: const Text('Decoders'),
        onTap: () => _settings('decoders'),
      ),
      ListTile(
        leading: const Icon(Icons.terminal),
        title: const Text('Automation API'),
        onTap: () => _settings('api'),
      ),
    ],
  );

  Widget _empty() => Center(
    child: Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            Icons.travel_explore,
            size: 48,
            color: Theme.of(context).colorScheme.outline,
          ),
          const SizedBox(height: 16),
          Text(
            _search.text.isEmpty
                ? 'Ready when you are'
                : 'No matching requests',
            style: Theme.of(context).textTheme.titleLarge,
          ),
          const SizedBox(height: 8),
          Text(
            _search.text.isEmpty
                ? 'Configure a device, then start eCapture.\nOr import an existing JSON or HAR capture.'
                : 'Try a different filter.',
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 20),
          Wrap(
            spacing: 12,
            children: [
              OutlinedButton.icon(
                onPressed: () => _settings(),
                icon: const Icon(Icons.add),
                label: const Text('Configure device'),
              ),
              TextButton(
                onPressed: _import,
                child: const Text('Import capture'),
              ),
            ],
          ),
        ],
      ),
    ),
  );

  Widget _tableHeader() => Container(
    height: 40,
    color: Theme.of(context).colorScheme.surfaceContainerLow,
    padding: const EdgeInsets.symmetric(horizontal: 16),
    child: const Row(
      children: [
        SizedBox(width: 64, child: Text('Method')),
        SizedBox(width: 52, child: Text('Status')),
        Expanded(child: Text('URL')),
        SizedBox(width: 74, child: Text('Protocol')),
        SizedBox(width: 64, child: Text('Size')),
        SizedBox(width: 72, child: Text('Time')),
      ],
    ),
  );

  Widget _requestRow(Json item, bool compact) {
    final method = item['method'] as String? ?? 'RAW';
    final status = item['status'] as int? ?? 0;
    final color = status >= 400
        ? Theme.of(context).colorScheme.error
        : Theme.of(context).colorScheme.primary;
    final url = item['url'] as String? ?? '';
    final size = base64Decode(
      (item['response'] as Json?)?['body'] as String? ?? '',
    ).length;
    final row = compact
        ? ListTile(
            leading: SizedBox(
              width: 56,
              child: Text(
                method,
                style: TextStyle(color: color, fontWeight: FontWeight.w700),
              ),
            ),
            title: Text(
              url.isEmpty ? item['connection_id'].toString() : url,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
            ),
            subtitle: Text(
              '${item['protocol']}  ·  ${item['device_id']}  ·  ${item['duration_ms']} ms',
            ),
            trailing: Text(
              status == 0 ? '…' : '$status',
              style: TextStyle(color: color),
            ),
            selected: _selected?['id'] == item['id'],
            onTap: () => _select(item, compact),
          )
        : InkWell(
            onTap: () => _select(item, compact),
            child: Container(
              height: 46,
              padding: const EdgeInsets.symmetric(horizontal: 16),
              color: _selected?['id'] == item['id']
                  ? Theme.of(context).colorScheme.primaryContainer
                  : null,
              child: Row(
                children: [
                  SizedBox(
                    width: 64,
                    child: Text(
                      method,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        color: color,
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                  SizedBox(
                    width: 52,
                    child: Text(
                      status == 0 ? '—' : '$status',
                      style: TextStyle(
                        color: color,
                        fontFeatures: const [FontFeature.tabularFigures()],
                      ),
                    ),
                  ),
                  Expanded(
                    child: Tooltip(
                      message: url,
                      child: Text(
                        url.isEmpty ? item['connection_id'].toString() : url,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                  ),
                  SizedBox(
                    width: 74,
                    child: Text(
                      item['protocol'] as String? ?? '',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ),
                  SizedBox(
                    width: 64,
                    child: Text(
                      size < 1024
                          ? '$size B'
                          : '${(size / 1024).toStringAsFixed(1)} KB',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ),
                  SizedBox(
                    width: 72,
                    child: Text(
                      '${item['duration_ms']} ms',
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ),
                ],
              ),
            ),
          );
    return Column(
      children: [
        row,
        Divider(
          height: 1,
          color: Theme.of(
            context,
          ).colorScheme.outlineVariant.withValues(alpha: .45),
        ),
      ],
    );
  }

  Widget _statusbar() => Container(
    height: 36,
    padding: const EdgeInsets.symmetric(horizontal: 16),
    color: Theme.of(context).colorScheme.surfaceContainerLow,
    child: Row(
      children: [
        Expanded(
          child: Text(
            '$_total requests · latest 1,000 retained',
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ),
        Icon(_live ? Icons.sync : Icons.pause, size: 14),
        const SizedBox(width: 6),
        Text(
          _live ? 'Live' : 'Paused',
          style: Theme.of(context).textTheme.bodySmall,
        ),
      ],
    ),
  );
}

class SessionsPage extends StatefulWidget {
  const SessionsPage({
    super.key,
    required this.api,
    required this.device,
    required this.decoders,
  });
  final LensApi api;
  final String? device;
  final List<Json> decoders;
  @override
  State<SessionsPage> createState() => _SessionsPageState();
}

class _SessionsPageState extends State<SessionsPage> {
  late Future<dynamic> _sessions;
  @override
  void initState() {
    super.initState();
    _sessions = widget.api.call('sessions.list', {
      if (widget.device != null) 'device_id': widget.device,
    });
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Sessions & timeline')),
    body: FutureBuilder<dynamic>(
      future: _sessions,
      builder: (context, snapshot) {
        if (snapshot.hasError) {
          return Center(child: SelectableText('${snapshot.error}'));
        }
        if (!snapshot.hasData) {
          return const Center(child: CircularProgressIndicator());
        }
        final sessions = (snapshot.data as List).cast<Json>();
        if (sessions.isEmpty) {
          return const Center(
            child: Text('Sessions appear after traffic is captured.'),
          );
        }
        return ListView.separated(
          itemCount: sessions.length,
          separatorBuilder: (_, _) => const Divider(height: 1),
          itemBuilder: (context, i) {
            final session = sessions[i];
            return ListTile(
              leading: const Icon(Icons.account_tree_outlined),
              title: Text('${session['connection_id']}'),
              subtitle: Text(
                '${session['device_id']} · ${session['count'] ?? session['requests'] ?? ''} requests',
              ),
              trailing: const Icon(Icons.chevron_right),
              onTap: () {
                Navigator.push(
                  context,
                  MaterialPageRoute<void>(
                    builder: (_) => TimelinePage(
                      api: widget.api,
                      session: session,
                      decoders: widget.decoders,
                    ),
                  ),
                );
              },
            );
          },
        );
      },
    ),
  );
}

class TimelinePage extends StatelessWidget {
  const TimelinePage({
    super.key,
    required this.api,
    required this.session,
    required this.decoders,
  });
  final LensApi api;
  final Json session;
  final List<Json> decoders;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Connection timeline')),
    body: FutureBuilder<dynamic>(
      future: api.call('sessions.timeline', {
        'connection_id': session['connection_id'],
        'device_id': session['device_id'],
      }),
      builder: (context, snapshot) {
        if (snapshot.hasError) {
          return Center(child: SelectableText('${snapshot.error}'));
        }
        if (!snapshot.hasData) {
          return const Center(child: CircularProgressIndicator());
        }
        final items = (snapshot.data as List).cast<Json>();
        return ListView.builder(
          itemCount: items.length,
          itemBuilder: (context, i) {
            final item = items[i];
            return ListTile(
              leading: const Icon(Icons.circle_outlined),
              title: Text('${item['method']} ${item['url']}'),
              subtitle: Text(
                '${item['started_at']}\nstream ${item['stream_id']} · ${item['duration_ms']} ms',
              ),
              isThreeLine: true,
              trailing: Text('${item['status']}'),
              onTap: () {
                Navigator.push(
                  context,
                  MaterialPageRoute<void>(
                    builder: (_) => RequestDetailPage(
                      api: api,
                      initial: item,
                      decoders: decoders,
                    ),
                  ),
                );
              },
            );
          },
        );
      },
    ),
  );
}
