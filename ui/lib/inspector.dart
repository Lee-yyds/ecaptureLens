import 'dart:async';
import 'dart:convert';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'api.dart';
import 'files.dart';

class Inspector extends StatefulWidget {
  const Inspector({
    super.key,
    required this.api,
    required this.transaction,
    required this.decoders,
  });
  final LensApi api;
  final Json transaction;
  final List<Json> decoders;
  @override
  State<Inspector> createState() => _InspectorState();
}

class _InspectorState extends State<Inspector> {
  String _side = 'response';
  String _part = 'body';
  String _format = 'text';
  String? _descriptor;
  String? _descriptorName;
  final _messageType = TextEditingController();
  String _text = '';
  String? _error;
  bool _loading = false;
  int _generation = 0;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void didUpdateWidget(covariant Inspector oldWidget) {
    super.didUpdateWidget(oldWidget);
    final formats = [
      'text',
      'json',
      'hex',
      'protobuf',
      'grpc',
      ...widget.decoders.map((d) => d['name'] as String),
    ];
    final removed = !formats.contains(_format);
    if (removed) _format = 'text';
    if (removed ||
        jsonEncode(oldWidget.transaction) != jsonEncode(widget.transaction)) {
      _load();
    }
  }

  Future<void> _load() async {
    final generation = ++_generation;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final message = widget.transaction[_side] as Json?;
      final String text;
      if (message == null) {
        text = _side == 'response'
            ? 'Waiting for a response…'
            : 'No request was captured.';
      } else if (_part == 'headers') {
        text = (message['headers'] as Json).entries
            .expand(
              (entry) =>
                  (entry.value as List).map((value) => '${entry.key}: $value'),
            )
            .join('\n');
      } else if (_part == 'raw') {
        text = utf8.decode(
          base64Decode(message['raw'] as String? ?? ''),
          allowMalformed: true,
        );
      } else {
        final result =
            await widget.api.call('body.view', {
                  'id': widget.transaction['id'],
                  'side': _side,
                  'format': _format,
                  if (_descriptor != null) 'descriptor': _descriptor,
                  if (_messageType.text.isNotEmpty)
                    'message_type': _messageType.text,
                })
                as Json;
        text = result['text'] as String;
      }
      if (mounted && generation == _generation) {
        setState(() {
          _text = text;
          _loading = false;
        });
      }
    } catch (error) {
      if (mounted && generation == _generation) {
        setState(() {
          _error = '$error';
          _loading = false;
        });
      }
    }
  }

  Future<void> _chooseDescriptor() async {
    try {
      final file = await FilePicker.platform.pickFiles(withData: true);
      if (file == null) return;
      final picked = file.files.single;
      final bytes = picked.bytes!;
      setState(() {
        _descriptor = base64Encode(bytes);
        _descriptorName = picked.name;
      });
      await _load();
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    }
  }

  Future<void> _export() async {
    try {
      final message = widget.transaction[_side] as Json?;
      if (message == null) throw LensError('No $_side body has been captured.');
      await saveBytes(
        '$_side.bin',
        base64Decode(message[_part == 'raw' ? 'raw' : 'body'] as String? ?? ''),
      );
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    }
  }

  @override
  void dispose() {
    _messageType.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final item = widget.transaction;
    final formats = [
      'text',
      'json',
      'hex',
      'protobuf',
      'grpc',
      ...widget.decoders.map((d) => d['name'] as String),
    ];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Text(
                    '${item['method']}',
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.primary,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(width: 10),
                  Text('${item['status'] == 0 ? 'Pending' : item['status']}'),
                  const Spacer(),
                  Text(
                    '${item['protocol']}',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ],
              ),
              const SizedBox(height: 10),
              SelectableText(
                '${item['url']}',
                style: Theme.of(context).textTheme.titleSmall,
              ),
              if ((item['src'] as String? ?? '').isNotEmpty ||
                  (item['dst'] as String? ?? '').isNotEmpty)
                SelectableText(
                  '${item['src'] ?? ''} → ${item['dst'] ?? ''}',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              const SizedBox(height: 10),
              Text(
                '${item['device_id']} · ${item['duration_ms']} ms · Stream ${item['stream_id'] ?? '—'}',
                style: Theme.of(context).textTheme.bodySmall,
              ),
              Text(
                '${item['started_at']}',
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: SegmentedButton<String>(
            segments: const [
              ButtonSegment(value: 'request', label: Text('Request')),
              ButtonSegment(value: 'response', label: Text('Response')),
            ],
            selected: {_side},
            onSelectionChanged: (value) {
              setState(() => _side = value.first);
              _load();
            },
          ),
        ),
        const SizedBox(height: 8),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: Row(
            children: [
              Expanded(
                child: SingleChildScrollView(
                  scrollDirection: Axis.horizontal,
                  child: SegmentedButton<String>(
                    segments: const [
                      ButtonSegment(value: 'body', label: Text('Body')),
                      ButtonSegment(value: 'headers', label: Text('Headers')),
                      ButtonSegment(value: 'raw', label: Text('Raw')),
                    ],
                    selected: {_part},
                    onSelectionChanged: (value) {
                      setState(() => _part = value.first);
                      _load();
                    },
                  ),
                ),
              ),
              IconButton(
                onPressed: _loading || _error != null
                    ? null
                    : () async {
                        await Clipboard.setData(ClipboardData(text: _text));
                        if (context.mounted) {
                          ScaffoldMessenger.of(context).showSnackBar(
                            const SnackBar(
                              content: Text('Copied to clipboard'),
                            ),
                          );
                        }
                      },
                icon: const Icon(Icons.copy_outlined),
                tooltip: 'Copy displayed content',
              ),
              IconButton(
                onPressed: _export,
                icon: const Icon(Icons.file_download_outlined),
                tooltip: 'Export original bytes',
              ),
            ],
          ),
        ),
        if (_part == 'body')
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
            child: Row(
              children: [
                const Text('Decode as'),
                const SizedBox(width: 16),
                Expanded(
                  child: DropdownButton<String>(
                    value: _format,
                    isExpanded: true,
                    items: formats
                        .toSet()
                        .map(
                          (format) => DropdownMenuItem(
                            value: format,
                            child: Text(
                              format == 'grpc'
                                  ? 'gRPC'
                                  : format == 'protobuf'
                                  ? 'Protobuf'
                                  : format.toUpperCase(),
                            ),
                          ),
                        )
                        .toList(),
                    onChanged: (value) {
                      setState(() => _format = value!);
                      _load();
                    },
                  ),
                ),
              ],
            ),
          ),
        if (_part == 'body' && ['protobuf', 'grpc'].contains(_format))
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                TextField(
                  controller: _messageType,
                  decoration: const InputDecoration(
                    labelText: 'Message type (optional)',
                    hintText: 'package.Message',
                  ),
                  onSubmitted: (_) => _load(),
                ),
                Row(
                  children: [
                    TextButton.icon(
                      onPressed: _chooseDescriptor,
                      icon: const Icon(Icons.attach_file),
                      label: const Text('Descriptor set'),
                    ),
                    if (_descriptorName != null)
                      Expanded(
                        child: Text(
                          _descriptorName!,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    if (_descriptor != null)
                      IconButton(
                        onPressed: () {
                          setState(() {
                            _descriptor = null;
                            _descriptorName = null;
                          });
                          _load();
                        },
                        icon: const Icon(Icons.close),
                        tooltip: 'Remove descriptor',
                      ),
                    IconButton(
                      onPressed: _load,
                      icon: const Icon(Icons.refresh),
                      tooltip: 'Decode',
                    ),
                  ],
                ),
              ],
            ),
          ),
        const Divider(height: 1),
        Expanded(
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : SingleChildScrollView(
                  padding: const EdgeInsets.all(16),
                  child: SelectableText(
                    _error ?? (_text.isEmpty ? '(empty)' : _text),
                    style: TextStyle(
                      fontFamily: 'monospace',
                      fontSize: 12,
                      height: 1.65,
                      color: _error != null
                          ? Theme.of(context).colorScheme.error
                          : null,
                    ),
                  ),
                ),
        ),
      ],
    );
  }
}

class RequestDetailPage extends StatefulWidget {
  const RequestDetailPage({
    super.key,
    required this.api,
    required this.initial,
    required this.decoders,
  });
  final LensApi api;
  final Json initial;
  final List<Json> decoders;
  @override
  State<RequestDetailPage> createState() => _RequestDetailPageState();
}

class _RequestDetailPageState extends State<RequestDetailPage> {
  late Json _transaction = widget.initial;
  late List<Json> _decoders = widget.decoders;
  Timer? _timer;
  String? _error;
  bool _refreshing = false;

  @override
  void initState() {
    super.initState();
    _timer = Timer.periodic(const Duration(seconds: 1), (_) => _refresh());
  }

  Future<void> _refresh() async {
    if (_refreshing) return;
    _refreshing = true;
    try {
      final result = await Future.wait([
        widget.api.call('requests.get', {'id': widget.initial['id']}),
        widget.api.call('decoders.list'),
      ]);
      if (mounted) {
        setState(() {
          _transaction = result[0] as Json;
          _decoders = (result[1] as List).cast<Json>();
          _error = null;
        });
      }
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    } finally {
      _refreshing = false;
    }
  }

  Future<void> _export() async {
    try {
      final result =
          await widget.api.call('history.export', {
                'format': 'json',
                'ids': [widget.initial['id']],
              })
              as Json;
      await saveBytes('request.json', utf8.encode(result['content'] as String));
    } catch (error) {
      if (mounted) setState(() => _error = '$error');
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: Text(_transaction['method'] as String? ?? 'Request'),
      actions: [
        IconButton(
          onPressed: _export,
          icon: const Icon(Icons.file_download_outlined),
          tooltip: 'Export request',
        ),
      ],
    ),
    body: SafeArea(
      top: false,
      child: Column(
        children: [
          if (_error != null)
            Padding(
              padding: const EdgeInsets.all(16),
              child: SelectableText(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
          Expanded(
            child: Inspector(
              api: widget.api,
              transaction: _transaction,
              decoders: _decoders,
            ),
          ),
        ],
      ),
    ),
  );
}
