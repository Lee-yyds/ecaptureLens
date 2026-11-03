import 'dart:convert';
import 'dart:ffi';
import 'dart:io';

import 'package:ffi/ffi.dart';
import 'package:path_provider/path_provider.dart';
import 'package:flutter/services.dart';

import 'api.dart';

class NativeCore {
  NativeCore._(this._library);
  final DynamicLibrary _library;
  static const _service = MethodChannel('ecapturelens/core_service');
  late final _free = _library
      .lookupFunction<
        Void Function(Pointer<Utf8>),
        void Function(Pointer<Utf8>)
      >('LensFree');

  static Future<(NativeCore, LensApi)> start(String directory) async {
    final String path;
    final override = Platform.environment['ECAPTURELENS_LIBRARY'];
    if (override != null) {
      path = override;
    } else if (Platform.isAndroid || Platform.isLinux) {
      path = 'libecapturelens.so';
    } else if (Platform.isWindows) {
      path = 'ecapturelens.dll';
    } else if (Platform.isMacOS) {
      path =
          '${File(Platform.resolvedExecutable).parent.parent.path}'
          '/Frameworks/libecapturelens.dylib';
    } else {
      throw UnsupportedError('This platform has no embedded Go core.');
    }
    final core = NativeCore._(DynamicLibrary.open(path));
    final start = core._library
        .lookupFunction<
          Pointer<Utf8> Function(Pointer<Utf8>),
          Pointer<Utf8> Function(Pointer<Utf8>)
        >('LensStart');
    final input = directory.toNativeUtf8();
    try {
      final result = core._read(start(input));
      if (Platform.isAndroid) {
        try {
          await _service.invokeMethod<void>('start');
        } catch (_) {
          await core.stop();
          rethrow;
        }
      }
      return (
        core,
        LensApi(result['url'] as String, result['token'] as String),
      );
    } finally {
      malloc.free(input);
    }
  }

  Json _read(Pointer<Utf8> value) {
    try {
      final result = jsonDecode(value.toDartString()) as Json;
      if (result['error'] != null) throw LensError(result['error'].toString());
      return result;
    } finally {
      _free(value);
    }
  }

  Future<void> stop() async {
    final stop = _library
        .lookupFunction<Pointer<Utf8> Function(), Pointer<Utf8> Function()>(
          'LensStop',
        );
    try {
      _read(stop());
    } finally {
      if (Platform.isAndroid) await _service.invokeMethod<void>('stop');
    }
  }
}

class ConnectionSettings {
  ConnectionSettings(this.file, this.data);
  final File file;
  Json data;

  static Future<ConnectionSettings> load() async {
    final folder = await getApplicationSupportDirectory();
    await folder.create(recursive: true);
    final file = File('${folder.path}/connection.json');
    return ConnectionSettings(
      file,
      await file.exists() ? jsonDecode(await file.readAsString()) as Json : {},
    );
  }

  String get dataDirectory => '${file.parent.path}/history';

  Future<void> save(Json value) async {
    final temporary = File('${file.path}.tmp');
    await temporary.writeAsString(jsonEncode(value), flush: true);
    await temporary.rename(file.path);
    data = value;
  }
}
