import 'package:ecapturelens/api.dart';
import 'package:ecapturelens/main.dart';
import 'package:ecapturelens/inspector.dart';
import 'package:ecapturelens/settings.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class TestApi extends LensApi {
  TestApi() : super('http://test.invalid', '');
  final calls = <String>[];
  bool hasDevice = true;

  @override
  Future<dynamic> call(String method, [Json params = const {}]) async {
    calls.add(method);
    return switch (method) {
      'requests.list' => {
        'items': [transaction],
        'total': 1,
      },
      'devices.list' => [
        if (hasDevice)
          {
            'id': 'phone',
            'name': 'Test phone',
            'endpoint': 'ws://localhost:28257',
          },
      ],
      'capture.status' || 'decoders.list' => <Json>[],
      'body.view' => {'text': '{"test":true}'},
      _ => throw StateError('Unexpected operation $method'),
    };
  }

  static final transaction = <String, dynamic>{
    'id': '1',
    'device_id': 'phone',
    'connection_id': 'tcp1',
    'stream_id': '3',
    'protocol': 'HTTP/2',
    'method': 'GET',
    'url': 'https://fixture.test/profile',
    'status': 200,
    'duration_ms': 12,
    'started_at': '2026-11-02T12:00:00Z',
    'request': {'headers': <String, dynamic>{}, 'body': '', 'raw': ''},
    'response': {
      'headers': {
        'Content-Type': ['application/json'],
      },
      'body': 'eyJ0ZXN0Ijp0cnVlfQ==',
      'raw': '',
    },
  };
}

class DetailApi extends TestApi {
  bool complete = false;
  @override
  Future<dynamic> call(String method, [Json params = const {}]) async {
    if (method == 'requests.get') {
      return {
        ...TestApi.transaction,
        'response': complete ? TestApi.transaction['response'] : null,
      };
    }
    return super.call(method, params);
  }
}

void main() {
  testWidgets(
    'request details retain endpoints when authority is unavailable',
    (tester) async {
      final api = TestApi();
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: Inspector(
              api: api,
              transaction: {
                ...TestApi.transaction,
                'url': '/profile',
                'src': '127.0.0.1:41000',
                'dst': '127.0.0.1:443',
              },
              decoders: const [],
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('127.0.0.1:41000 → 127.0.0.1:443'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
      api.close();
    },
  );
  testWidgets('removing selected device resets its dropdown', (tester) async {
    final api = TestApi();
    await tester.pumpWidget(LensApp(client: api));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(DropdownButtonFormField<String>));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Test phone').last);
    await tester.pumpAndSettle();
    api.hasDevice = false;
    await tester.pump(const Duration(seconds: 1));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.text('Select device'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    api.close();
  });
  testWidgets('mobile pending details update when a response arrives', (
    tester,
  ) async {
    final api = DetailApi();
    await tester.pumpWidget(
      MaterialApp(
        home: RequestDetailPage(
          api: api,
          initial: {...TestApi.transaction, 'response': null},
          decoders: const [],
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Waiting for a response…'), findsOneWidget);
    api.complete = true;
    await tester.pump(const Duration(seconds: 1));
    await tester.pumpAndSettle();
    expect(find.text('{"test":true}'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    api.close();
  });

  testWidgets('API console provides a CLI connection with its token', (
    tester,
  ) async {
    final api = LensApi('http://127.0.0.1:12345', 'test-token');
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: ApiConsole(api: api)),
      ),
    );
    expect(find.text('Copy CLI connection'), findsOneWidget);
    expect(find.textContaining('--token'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    api.close();
  });

  testWidgets('removing selected decoder restores a valid body format', (
    tester,
  ) async {
    final api = TestApi();
    Widget render(List<Json> decoders) => MaterialApp(
      home: Scaffold(
        body: Inspector(
          api: api,
          transaction: TestApi.transaction,
          decoders: decoders,
        ),
      ),
    );
    await tester.pumpWidget(
      render([
        {'name': 'custom', 'command': 'cat'},
      ]),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byType(DropdownButton<String>));
    await tester.pumpAndSettle();
    await tester.tap(find.text('CUSTOM').last);
    await tester.pumpAndSettle();
    await tester.pumpWidget(render([]));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(
      tester
          .widget<DropdownButton<String>>(find.byType(DropdownButton<String>))
          .value,
      'text',
    );
    await tester.pumpWidget(const SizedBox());
    api.close();
  });
  testWidgets('desktop shows request and Go-backed inspector', (tester) async {
    tester.view.physicalSize = const Size(1440, 900);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final api = TestApi();
    await tester.pumpWidget(LensApp(client: api));
    await tester.pumpAndSettle();
    expect(find.text('Workspace'), findsOneWidget);
    expect(find.text('https://fixture.test/profile'), findsOneWidget);
    await tester.tap(find.text('https://fixture.test/profile'));
    await tester.pumpAndSettle();
    expect(find.text('{"test":true}'), findsOneWidget);
    expect(api.calls, contains('body.view'));
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    api.close();
  });

  testWidgets('phone opens details and system back returns to list', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final api = TestApi();
    await tester.pumpWidget(LensApp(client: api));
    await tester.pumpAndSettle();
    await tester.tap(find.text('https://fixture.test/profile'));
    await tester.pumpAndSettle();
    expect(find.text('Response'), findsOneWidget);
    expect(find.text('{"test":true}'), findsOneWidget);
    await tester.pageBack();
    await tester.pumpAndSettle();
    expect(find.text('Ready when you are'), findsNothing);
    expect(find.text('https://fixture.test/profile'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    api.close();
  });
}
