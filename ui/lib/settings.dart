import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'api.dart';

class SettingsPage extends StatefulWidget {
  const SettingsPage({
    super.key,
    required this.api,
    this.initialSection = 'devices',
  });
  final LensApi api;
  final String initialSection;
  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  late String _section;
  List<Json> _devices = [];
  List<Json> _decoders = [];
  List<Json> _statuses = [];
  String? _error;
  bool _busy = true;

  @override
  void initState() {
    super.initState();
    _section = widget.initialSection;
    _load();
  }

  Future<void> _load() async {
    try {
      final results = await Future.wait([
        widget.api.call('devices.list'),
        widget.api.call('decoders.list'),
        widget.api.call('capture.status'),
      ]);
      if (mounted) {
        setState(() {
          _devices = (results[0] as List).cast<Json>();
          _decoders = (results[1] as List).cast<Json>();
          _statuses = (results[2] as List).cast<Json>();
          _busy = false;
          _error = null;
        });
      }
    } catch (error) {
      if (mounted) {
        setState(() {
          _error = '$error';
          _busy = false;
        });
      }
    }
  }

  Future<void> _editDevice([Json? device]) async {
    await Navigator.push(
      context,
      MaterialPageRoute<void>(
        builder: (_) => DeviceEditor(api: widget.api, device: device),
      ),
    );
    await _load();
  }

  Future<void> _editDecoder([Json? decoder]) async {
    await Navigator.push(
      context,
      MaterialPageRoute<void>(
        builder: (_) => DecoderEditor(api: widget.api, decoder: decoder),
      ),
    );
    await _load();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Workspace settings'),
      actions: [
        IconButton(
          onPressed: _load,
          icon: const Icon(Icons.refresh),
          tooltip: 'Refresh settings',
        ),
      ],
    ),
    body: Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(16),
          child: SegmentedButton<String>(
            segments: const [
              ButtonSegment(
                value: 'devices',
                label: Text('Devices'),
                icon: Icon(Icons.smartphone_outlined),
              ),
              ButtonSegment(
                value: 'decoders',
                label: Text('Decoders'),
                icon: Icon(Icons.data_object),
              ),
              ButtonSegment(
                value: 'api',
                label: Text('API'),
                icon: Icon(Icons.terminal),
              ),
            ],
            selected: {_section},
            onSelectionChanged: (value) =>
                setState(() => _section = value.first),
          ),
        ),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.all(16),
            child: SelectableText(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ),
        Expanded(
          child: _busy
              ? const Center(child: CircularProgressIndicator())
              : _section == 'api'
              ? ApiConsole(api: widget.api)
              : _section == 'devices'
              ? _deviceList()
              : _decoderList(),
        ),
      ],
    ),
    floatingActionButton: _section == 'api'
        ? null
        : FloatingActionButton.extended(
            onPressed: () =>
                _section == 'devices' ? _editDevice() : _editDecoder(),
            icon: const Icon(Icons.add),
            label: Text(_section == 'devices' ? 'Add device' : 'Add decoder'),
          ),
  );

  Widget _deviceList() => ListView(
    padding: const EdgeInsets.fromLTRB(16, 0, 16, 96),
    children: [
      const Padding(
        padding: EdgeInsets.only(bottom: 16),
        child: Text(
          'Capture commands run on the device hosting the Go core. On Android use su; on a computer you can use adb -s SERIAL. Each device connects to its own eCaptureQ WebSocket endpoint.',
        ),
      ),
      OutlinedButton.icon(
        onPressed: () async {
          try {
            final result = await widget.api.call('devices.discover');
            if (mounted) {
              showDialog<void>(
                context: context,
                builder: (context) => AlertDialog(
                  title: const Text('ADB devices'),
                  content: SelectableText(
                    const JsonEncoder.withIndent('  ').convert(result),
                  ),
                  actions: [
                    TextButton(
                      onPressed: () => Navigator.pop(context),
                      child: const Text('Close'),
                    ),
                  ],
                ),
              );
            }
          } catch (error) {
            if (mounted) setState(() => _error = '$error');
          }
        },
        icon: const Icon(Icons.usb),
        label: const Text('Discover ADB devices'),
      ),
      if (_devices.isEmpty)
        const Padding(
          padding: EdgeInsets.symmetric(vertical: 40),
          child: Text(
            'No devices configured. Add one to set its capture endpoint and start/stop commands.',
            textAlign: TextAlign.center,
          ),
        ),
      for (final device in _devices) ...[
        ListTile(
          contentPadding: const EdgeInsets.symmetric(vertical: 8),
          leading: const Icon(Icons.smartphone_outlined),
          title: Text('${device['name']}'),
          subtitle: Text('${device['id']}\n${device['endpoint']}'),
          isThreeLine: true,
          trailing: IconButton(
            onPressed: () => _editDevice(device),
            icon: const Icon(Icons.edit_outlined),
            tooltip: 'Edit device',
          ),
        ),
        for (final status in _statuses.where(
          (s) => s['device_id'] == device['id'],
        ))
          Padding(
            padding: const EdgeInsets.only(left: 56, bottom: 16),
            child: SelectableText(
              '${status['state']}${status['error'] == null || status['error'] == '' ? '' : '\n${status['error']}'}${status['logs'] == null ? '' : '\n${status['logs']}'}',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ),
        const Divider(),
      ],
    ],
  );

  Widget _decoderList() => ListView(
    padding: const EdgeInsets.fromLTRB(16, 0, 16, 96),
    children: [
      const Text(
        'Built in: JSON, text, hex, Protobuf (wire or descriptor), gRPC. Add a business decoder command to receive original body bytes on stdin and return readable output on stdout. A nonzero exit is shown as an error.',
      ),
      const SizedBox(height: 20),
      for (final decoder in _decoders)
        ListTile(
          leading: const Icon(Icons.data_object),
          title: Text('${decoder['name']}'),
          subtitle: Text('${decoder['command']}'),
          trailing: IconButton(
            onPressed: () => _editDecoder(decoder),
            icon: const Icon(Icons.edit_outlined),
            tooltip: 'Edit decoder',
          ),
        ),
    ],
  );
}

class DeviceEditor extends StatefulWidget {
  const DeviceEditor({super.key, required this.api, this.device});
  final LensApi api;
  final Json? device;
  @override
  State<DeviceEditor> createState() => _DeviceEditorState();
}

class _DeviceEditorState extends State<DeviceEditor> {
  final _form = GlobalKey<FormState>();
  late final _id = TextEditingController(
    text: widget.device?['id'] as String? ?? 'android-local',
  );
  late final _name = TextEditingController(
    text: widget.device?['name'] as String? ?? 'Android',
  );
  late final _endpoint = TextEditingController(
    text: widget.device?['endpoint'] as String? ?? 'ws://127.0.0.1:28257',
  );
  late final _start = TextEditingController(
    text: widget.device?['start_command'] as String? ?? '',
  );
  late final _stop = TextEditingController(
    text: widget.device?['stop_command'] as String? ?? '',
  );
  String? _error;
  bool _busy = false;

  Future<void> _save() async {
    if (!_form.currentState!.validate()) return;
    setState(() => _busy = true);
    try {
      await widget.api.call('devices.put', {
        'id': _id.text,
        'name': _name.text,
        'endpoint': _endpoint.text,
        'start_command': _start.text,
        'stop_command': _stop.text,
      });
      if (mounted) Navigator.pop(context);
    } catch (error) {
      if (mounted) {
        setState(() {
          _error = '$error';
          _busy = false;
        });
      }
    }
  }

  Future<void> _delete() async {
    setState(() => _busy = true);
    try {
      await widget.api.call('devices.delete', {'id': _id.text});
      if (mounted) Navigator.pop(context);
    } catch (error) {
      if (mounted) {
        setState(() {
          _error = '$error';
          _busy = false;
        });
      }
    }
  }

  @override
  void dispose() {
    for (final c in [_id, _name, _endpoint, _start, _stop]) {
      c.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: Text(widget.device == null ? 'Add device' : 'Edit device'),
      actions: [
        if (widget.device != null)
          IconButton(
            onPressed: _busy ? null : _delete,
            icon: const Icon(Icons.delete_outline),
            tooltip: 'Delete device',
          ),
      ],
    ),
    body: Form(
      key: _form,
      child: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          TextFormField(
            controller: _id,
            enabled: widget.device == null,
            decoration: const InputDecoration(labelText: 'Device ID'),
            validator: (value) => value == null || value.trim().isEmpty
                ? 'Enter a unique device ID'
                : null,
          ),
          const SizedBox(height: 20),
          TextFormField(
            controller: _name,
            decoration: const InputDecoration(labelText: 'Display name'),
            validator: (value) =>
                value == null || value.trim().isEmpty ? 'Enter a name' : null,
          ),
          const SizedBox(height: 20),
          TextFormField(
            controller: _endpoint,
            decoration: const InputDecoration(
              labelText: 'eCaptureQ WebSocket endpoint',
              hintText: 'ws://127.0.0.1:28257',
            ),
          ),
          const SizedBox(height: 20),
          TextFormField(
            controller: _start,
            minLines: 3,
            maxLines: 8,
            decoration: const InputDecoration(
              labelText: 'Start capture command',
              hintText:
                  "su -c '/data/local/tmp/ecapture tls --text-source pcapng …'",
            ),
          ),
          const SizedBox(height: 20),
          TextFormField(
            controller: _stop,
            minLines: 2,
            maxLines: 6,
            decoration: const InputDecoration(
              labelText: 'Stop capture command',
              hintText: "su -c 'kill -INT CAPTURE_PID'",
            ),
          ),
          const SizedBox(height: 16),
          const Text(
            'Use your installed eCapture version’s --ecaptureq option to publish reconstructed text events. Commands are explicit shell commands; retain a PID in your start command for precise cleanup. Leave commands empty to connect to an already-running capture.',
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 20),
              child: SelectableText(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
          const SizedBox(height: 24),
          FilledButton.icon(
            onPressed: _busy ? null : _save,
            icon: const Icon(Icons.check),
            label: Text(_busy ? 'Saving…' : 'Save device'),
          ),
        ],
      ),
    ),
  );
}

class DecoderEditor extends StatefulWidget {
  const DecoderEditor({super.key, required this.api, this.decoder});
  final LensApi api;
  final Json? decoder;
  @override
  State<DecoderEditor> createState() => _DecoderEditorState();
}

class _DecoderEditorState extends State<DecoderEditor> {
  late final _name = TextEditingController(
    text: widget.decoder?['name'] as String? ?? '',
  );
  late final _command = TextEditingController(
    text: widget.decoder?['command'] as String? ?? '',
  );
  String? _error;
  bool _busy = false;

  Future<void> _submit(bool delete) async {
    setState(() => _busy = true);
    try {
      await widget.api.call(delete ? 'decoders.delete' : 'decoders.put', {
        'name': _name.text,
        if (!delete) 'command': _command.text,
      });
      if (mounted) Navigator.pop(context);
    } catch (error) {
      if (mounted) {
        setState(() {
          _error = '$error';
          _busy = false;
        });
      }
    }
  }

  @override
  void dispose() {
    _name.dispose();
    _command.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Business decoder'),
      actions: [
        if (widget.decoder != null)
          IconButton(
            onPressed: _busy ? null : () => _submit(true),
            icon: const Icon(Icons.delete_outline),
            tooltip: 'Delete decoder',
          ),
      ],
    ),
    body: ListView(
      padding: const EdgeInsets.all(24),
      children: [
        TextField(
          controller: _name,
          enabled: widget.decoder == null,
          decoration: const InputDecoration(labelText: 'Decoder name'),
        ),
        const SizedBox(height: 20),
        TextField(
          controller: _command,
          minLines: 3,
          maxLines: 8,
          decoration: const InputDecoration(
            labelText: 'Command',
            hintText: 'python3 /path/to/decode.py',
          ),
        ),
        const SizedBox(height: 16),
        const Text(
          'The command runs on the core host, reads the body from stdin, and writes decoded text to stdout. It appears in the inspector’s decoder menu and in the CLI.',
        ),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 20),
            child: SelectableText(
              _error!,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ),
        const SizedBox(height: 24),
        FilledButton(
          onPressed: _busy ? null : () => _submit(false),
          child: const Text('Save decoder'),
        ),
      ],
    ),
  );
}

class ApiConsole extends StatefulWidget {
  const ApiConsole({super.key, required this.api});
  final LensApi api;
  @override
  State<ApiConsole> createState() => _ApiConsoleState();
}

class _ApiConsoleState extends State<ApiConsole> {
  final _method = TextEditingController(text: 'api.describe');
  final _params = TextEditingController(text: '{}');
  String _output = '';
  String? _error;
  bool _busy = false;

  String get _connectionCommand => [
    'ecapturelens',
    '--server',
    widget.api.url,
    if (widget.api.token.isNotEmpty) ...['--token', widget.api.token],
    'api',
    'describe',
  ].map((value) => "'${value.replaceAll("'", "'\\''")}'").join(' ');
  Future<void> _call() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final value = await widget.api.call(
        _method.text,
        jsonDecode(_params.text) as Json,
      );
      if (mounted) {
        setState(
          () => _output = const JsonEncoder.withIndent('  ').convert(value),
        );
      }
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  void dispose() {
    _method.dispose();
    _params.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => ListView(
    padding: const EdgeInsets.all(16),
    children: [
      SelectableText(
        'Service: ${widget.api.url}\nAll CLI and UI operations share this JSON API.',
      ),
      const SizedBox(height: 12),
      SelectableText(
        _connectionCommand,
        style: const TextStyle(fontFamily: 'monospace'),
      ),
      Align(
        alignment: Alignment.centerLeft,
        child: TextButton.icon(
          onPressed: () =>
              Clipboard.setData(ClipboardData(text: _connectionCommand)),
          icon: const Icon(Icons.terminal),
          label: const Text('Copy CLI connection'),
        ),
      ),
      const Text(
        'The command includes this service’s access token. To connect from a computer to a phone’s local service, first forward its port with adb forward tcp:PORT tcp:PORT.',
      ),
      const SizedBox(height: 20),
      TextField(
        controller: _method,
        decoration: const InputDecoration(labelText: 'Method'),
      ),
      const SizedBox(height: 16),
      TextField(
        controller: _params,
        minLines: 3,
        maxLines: 8,
        decoration: const InputDecoration(labelText: 'Parameters (JSON)'),
      ),
      const SizedBox(height: 16),
      Row(
        children: [
          FilledButton.icon(
            onPressed: _busy ? null : _call,
            icon: const Icon(Icons.play_arrow),
            label: Text(_busy ? 'Running…' : 'Run operation'),
          ),
          const SizedBox(width: 12),
          TextButton.icon(
            onPressed: () => Clipboard.setData(ClipboardData(text: _output)),
            icon: const Icon(Icons.copy),
            label: const Text('Copy result'),
          ),
        ],
      ),
      const SizedBox(height: 20),
      SelectableText(
        _error ?? _output,
        style: TextStyle(
          fontFamily: 'monospace',
          color: _error == null ? null : Theme.of(context).colorScheme.error,
        ),
      ),
    ],
  );
}
