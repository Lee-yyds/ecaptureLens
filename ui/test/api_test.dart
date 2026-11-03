@TestOn('vm')
library;

import 'dart:convert';
import 'dart:io';

import 'package:ecapturelens/api.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('API sends bearer token and surfaces gateway error', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final api = LensApi('http://127.0.0.1:${server.port}', 'secret');
    addTearDown(() async {
      api.close();
      await server.close(force: true);
    });
    server.listen((request) async {
      expect(request.headers.value('authorization'), 'Bearer secret');
      final body = jsonDecode(await utf8.decoder.bind(request).join()) as Json;
      expect(body['method'], 'requests.get');
      request.response.headers.contentType = ContentType.json;
      request.response.write(jsonEncode({'error': 'request not found'}));
      await request.response.close();
    });
    await expectLater(
      api.call('requests.get', {'id': 'missing'}),
      throwsA(isA<LensError>()),
    );
  });
}
