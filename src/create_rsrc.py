"""Generate Windows icon .syso (COFF .rsrc) without third-party build tools.
Contains 32px, 64px and 256px icon PNG resources plus icon group ID 1.
"""
import struct, pathlib
p=pathlib.Path(__file__).parent
ico=(p/'Rust_TC_Studio.ico').read_bytes()
reserved, typ, count=struct.unpack_from('<HHH',ico,0)
assert (reserved,typ)==(0,1)
imgs={}
for i in range(count):
    w,h,colors,res,planes,bits,nbytes,ofs=struct.unpack_from('<BBBBHHII',ico,6+i*16)
    sz=w or 256
    if sz in (32,64,256): imgs[sz]=(planes,bits,ico[ofs:ofs+nbytes])
assert len(imgs)==3,imgs.keys()
widths=[32,64,256]
buf=bytearray(256)
def u16(off,x): struct.pack_into('<H',buf,off,x)
def u32(off,x): struct.pack_into('<I',buf,off,x)
def dir_at(off,n):
    u16(off+12,0);u16(off+14,n)
# Root type directories: RT_ICON=3, RT_GROUP_ICON=14
dir_at(0,2)
u32(16,3);u32(20,0x80000000|32)
u32(24,14);u32(28,0x80000000|72)
# RT_ICON: three icon IDs (1..3)
dir_at(32,3)
for i in range(3):
    u32(48+i*8,i+1);u32(52+i*8,0x80000000|(96+i*24))
# RT_GROUP_ICON: one icon group ID 1
dir_at(72,1);u32(88,1);u32(92,0x80000000|168)
# Each language directory has exactly one lang 0x409 data entry
for i in range(3):
    off=96+i*24
    dir_at(off,1)
    u32(off+16,0x409);u32(off+20,192+16*i)
dir_at(168,1);u32(184,0x409);u32(188,240)
# ICON GROUP structure referencing resource IDs
raw_group=bytearray(struct.pack('<HHH',0,1,3))
for i,w in enumerate(widths):
    planes,bits,raw=imgs[w]
    raw_group+=struct.pack('<BBBBHHIH',w%256,w%256,0,0,planes,bits,len(raw),i+1)
assert len(raw_group)==48
# Add group then icon raw resources, 4-byte aligned
offsets={}
for key,data in [('group',raw_group)]+[(w,imgs[w][2]) for w in widths]:
    while len(buf)%4:buf.append(0)
    offsets[key]=len(buf)
    buf+=data
for i,w in enumerate(widths):
    ofs=192+i*16
    u32(ofs,offsets[w]) # relocated + .rsrc RVA
    u32(ofs+4,len(imgs[w][2]))
    u32(ofs+8,0);u32(ofs+12,0)
u32(240,offsets['group'])
u32(244,len(raw_group));u32(248,0);u32(252,0)
# IMAGE_FILE_HEADER(20), SECTION_HEADER(40), raw data, relocations, 1 symbol (18), string table (4)
header_size=20+40
raw_off=header_size
rel_off=raw_off+len(buf)
sym_off=rel_off+40
coff=bytearray(struct.pack('<HHIIIHH',0x8664,1,0,sym_off,1,0,0))
coff+=struct.pack('<8sIIIIIIHHI',b'.rsrc\0\0\0',len(buf),0,len(buf),raw_off,rel_off,0,4,0,0x40000040)
coff+=buf
# 4 relocations IMAGE_REL_AMD64_ADDR32NB, to .rsrc section symbol #0.
for ofs in (192,208,224,240):coff+=struct.pack('<IIH',ofs,0,3)
coff+=struct.pack('<8sIhHBB',b'.rsrc\0\0\0',0,1,0,3,0)
coff+=struct.pack('<I',4)
(p/'rsrc_windows_amd64.syso').write_bytes(coff)
print('resource object',len(coff),'icon entries',[(w,len(imgs[w][2])) for w in widths])
