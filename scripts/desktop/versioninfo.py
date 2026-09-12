"""Attach Windows product/version resources before hashing or signing Go clients."""
import argparse
import ctypes
import os
from pathlib import Path
import re
import struct


DESCRIPTIONS = {
    'nlroom-cli.exe': 'NodeLane Room Command Line',
    'nlroom-service.exe': 'NodeLane Room Network Service',
    'nlroom-update.exe': 'NodeLane Room Setup and Update',
}


def version_resource(version, filename):
    if not re.fullmatch(r'\d+\.\d+\.\d+', version):
        raise ValueError('Use the numeric client release version')
    major, minor, patch = map(int, version.split('.'))
    if max(major, minor, patch) > 65535:
        raise ValueError('Windows version components must fit in 16 bits')
    if filename not in DESCRIPTIONS:
        raise ValueError('Expected a Go Windows client executable')

    def block(key, value=b'', children=(), text=False):
        data = bytearray(struct.pack('<HHH', 0, len(value) // 2 if text else len(value), int(text)))
        data += (key + '\0').encode('utf-16le')
        data += b'\0' * (-len(data) % 4)
        data += value
        for child in children:
            data += b'\0' * (-len(data) % 4)
            data += child
        struct.pack_into('<H', data, 0, len(data))
        return data

    strings = {
        'CompanyName': 'NodeLane', 'FileDescription': DESCRIPTIONS[filename],
        'FileVersion': version, 'InternalName': filename[:-4],
        'LegalCopyright': 'NodeLane', 'OriginalFilename': filename,
        'ProductName': 'NodeLane Room', 'ProductVersion': version,
    }
    table = block('040904B0', children=[block(key, (value + '\0').encode('utf-16le'), text=True)
                                      for key, value in strings.items()], text=True)
    translation = block('Translation', struct.pack('<HH', 0x0409, 1200))
    # VS_FIXEDFILEINFO: NT/Win32 application, with matching file/product versions.
    ms, ls = major << 16 | minor, patch << 16
    fixed = struct.pack('<13I', 0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x00040004, 1, 0, 0, 0)
    return bytes(block('VS_VERSION_INFO', fixed, [block('StringFileInfo', children=[table], text=True),
                                                block('VarFileInfo', children=[translation], text=True)]))


def write_version_resource(path, version):
    if os.name != 'nt':
        raise SystemExit('Windows release metadata requires the Windows resource API')
    data = version_resource(version, path.name)
    kernel = ctypes.WinDLL('kernel32', use_last_error=True)
    kernel.BeginUpdateResourceW.argtypes = [ctypes.c_wchar_p, ctypes.c_int]
    kernel.BeginUpdateResourceW.restype = ctypes.c_void_p
    kernel.UpdateResourceW.argtypes = [ctypes.c_void_p, ctypes.c_void_p, ctypes.c_void_p,
                                      ctypes.c_ushort, ctypes.c_void_p, ctypes.c_uint32]
    kernel.UpdateResourceW.restype = ctypes.c_int
    kernel.EndUpdateResourceW.argtypes = [ctypes.c_void_p, ctypes.c_int]
    kernel.EndUpdateResourceW.restype = ctypes.c_int
    handle = kernel.BeginUpdateResourceW(str(path.resolve()), False)
    if not handle:
        raise ctypes.WinError(ctypes.get_last_error())
    buffer = ctypes.create_string_buffer(data)
    if not kernel.UpdateResourceW(handle, 16, 1, 0x0409, buffer, len(data)):
        error = ctypes.get_last_error()
        kernel.EndUpdateResourceW(handle, True)
        raise ctypes.WinError(error)
    if not kernel.EndUpdateResourceW(handle, False):
        raise ctypes.WinError(ctypes.get_last_error())


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('file', type=Path)
    args = parser.parse_args()
    write_version_resource(args.file, args.version)
